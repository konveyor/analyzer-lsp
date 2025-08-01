package java

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
)

func (p *javaProvider) StartJDTLS(ctx context.Context, lspServerPath, workspace, jvmMaxMem string) (io.Reader, io.Writer, chan (error), error) {
	jdtlsBasePath, err := filepath.Abs(filepath.Dir(filepath.Dir(lspServerPath)))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed finding jdtls base path - %w", err)
	}

	sharedConfigPath, err := getSharedConfigPath(jdtlsBasePath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get shared config path - %w", err)
	}

	jarPath, err := findEquinoxLauncher(jdtlsBasePath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to find equinox launcher - %w", err)
	}

	javaExec, err := getJavaExecutable(true)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed getting java executable - %v", err)
	}

	jdtlsArgs := []string{
		"-Declipse.application=org.eclipse.jdt.ls.core.id1",
		"-Dosgi.bundles.defaultStartLevel=4",
		"-Declipse.product=org.eclipse.jdt.ls.core.product",
		"-Dosgi.checkConfiguration=true",
		fmt.Sprintf("-Dosgi.sharedConfiguration.area=%s", sharedConfigPath),
		"-Dosgi.sharedConfiguration.area.readOnly=true",
		//"-agentlib:jdwp=transport=dt_socket,server=y,suspend=n,address=*:1044",
		"-Dosgi.configuration.cascaded=true",
		"-Xms1g",
		"-XX:MaxRAMPercentage=70.0",
		"--add-modules=ALL-SYSTEM",
		"--add-opens", "java.base/java.util=ALL-UNNAMED",
		"--add-opens", "java.base/java.lang=ALL-UNNAMED",
		"-jar", jarPath,
		"-Djava.net.useSystemProxies=true",
		"-configuration", "./",
		"-data", workspace,
	}

	if jvmMaxMem != "" {
		jdtlsArgs = append(jdtlsArgs, fmt.Sprintf("-Xmx%s", jvmMaxMem))
	}
	cmd := exec.CommandContext(ctx, javaExec, jdtlsArgs...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, err
	}

	waitErrorChannel := make(chan error)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	var returnErr error
	go func() {
		err := cmd.Start()
		wg.Done()
		if err != nil {
			returnErr = err
			p.Log.Error(err, "unable to  start lsp command")
			return
		}
		// Here we need to wait for the command to finish or if the ctx is cancelled,
		// To close the pipes.
		select {
		case err := <-waitErrorChannel:
			// language server has not started - don't error yet
			if err != nil && cmd.ProcessState == nil {
				p.Log.Info("retrying language server start")
			} else {
				p.Log.Error(err, "language server stopped with error")
			}
			p.Log.V(5).Info("language server stopped")
		case <-ctx.Done():
			p.Log.Info("language server context cancelled closing pipes")
			stdin.Close()
			stdout.Close()
		}
	}()

	// This will close the go routine above when wait has completed.
	go func() {
		waitErrorChannel <- cmd.Wait()
	}()

	wg.Wait()

	return stdout, stdin, waitErrorChannel, returnErr
}

func getSharedConfigPath(jdtlsBaseDir string) (string, error) {
	var configDir string
	switch runtime.GOOS {
	case "linux", "freebsd":
		configDir = "config_linux"
	case "darwin":
		configDir = "config_mac"
	case "windows":
		configDir = "config_win"
	default:
		return "", fmt.Errorf("unknown platform %s detected", runtime.GOOS)
	}
	return filepath.Join(jdtlsBaseDir, configDir), nil
}

func findEquinoxLauncher(jdtlsBaseDir string) (string, error) {
	pluginsDir := filepath.Join(jdtlsBaseDir, "plugins")
	files, err := os.ReadDir(pluginsDir)
	if err != nil {
		return "", fmt.Errorf("failed to read plugins directory: %w", err)
	}

	for _, file := range files {
		if strings.HasPrefix(file.Name(), "org.eclipse.equinox.launcher_") && strings.HasSuffix(file.Name(), ".jar") {
			return filepath.Join(pluginsDir, file.Name()), nil
		}
	}

	return "", errors.New("cannot find equinox launcher")
}

func getJavaExecutable(validateJavaVersion bool) (string, error) {
	javaExecutable := "java"
	if javaHome, exists := os.LookupEnv("JAVA_HOME"); exists {
		javaExecToTest := filepath.Join(javaHome, "bin", "java")
		if runtime.GOOS == "windows" {
			javaExecToTest += ".exe"
		}
		if _, err := os.Stat(javaExecToTest); err == nil {
			javaExecutable = javaExecToTest
		}
	}

	if !validateJavaVersion {
		return javaExecutable, nil
	}

	out, err := exec.Command(javaExecutable, "-version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to run %s -version: %w", javaExecutable, err)
	}

	re := regexp.MustCompile(`version\s"(\d+)[.\d]*"`)
	matches := re.FindStringSubmatch(string(out))
	if len(matches) > 1 {
		javaVersion := matches[1]
		if majorVersion := javaVersion; majorVersion < "17" {
			return "", errors.New("jdtls requires at least Java 17")
		}
		return javaExecutable, nil
	}

	return "", errors.New("could not determine Java version")
}
