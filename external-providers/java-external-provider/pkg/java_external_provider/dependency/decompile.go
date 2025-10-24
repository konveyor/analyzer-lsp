package dependency

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency/labels"
)

const (
	JavaFile          = ".java"
	JavaArchive       = ".jar"
	WebArchive        = ".war"
	EnterpriseArchive = ".ear"
	ClassFile         = ".class"
	MvnURIPrefix      = "mvn://"
	PomXmlFile        = "pom.xml"
)

const (
	METAINF = "META-INF"
	WEBINF  = "WEB-INF"
)

const (
	// File and directory permissions
	DirPermRWX    = 0755 // rwxr-xr-x: Owner can read/write/execute, others can read/execute
	DirPermRWXGrp = 0770 // rwxrwx---: Owner and group can read/write/execute
	FilePermRW    = 0644 // rw-r--r--: Owner can read/write, others can read
)

const (
	EMBEDDED_KONVEYOR_GROUP = "io.konveyor.embededdep"
	DefaultWorkerPoolSize   = 10 // Number of parallel workers for decompilation
)

type decompileFilter interface {
	shouldDecompile(JavaArtifact) bool
}

type alwaysDecompileFilter bool

func (a alwaysDecompileFilter) shouldDecompile(j JavaArtifact) bool {
	return bool(a)
}

type decompileJob interface {
	Run(ctx context.Context, log logr.Logger) error
	Done()
}

type baseArtifact struct {
	artifactPath        string
	m2Repo              string
	decompileTool       string
	javaPath            string
	labeler             labels.Labeler
	mavenIndexPath      string
	decompiler          internalDecompiler
	decompilerResponses chan DecomplierResponse
	decompilerWG        *sync.WaitGroup
}

func (b *baseArtifact) getFileName() string {
	name, _ := strings.CutSuffix(filepath.Base(b.artifactPath), ".jar")
	return name
}

func (b *baseArtifact) Done() {
	b.decompilerWG.Done()
}

func (b *baseArtifact) getM2Path(dep JavaArtifact) string {
	// Gives us the filepath parts from the group.
	groupParts := strings.Split(dep.GroupId, ".")
	// Gets us the filepath representation for the group
	groupFilePath := filepath.Join(groupParts...)

	// Destination for this file during copy always goes to the m2Repo.
	return filepath.Join(b.m2Repo, groupFilePath, dep.ArtifactId, dep.Version)
}

func (b *baseArtifact) getDecompileCommand(ctx context.Context, artifactPath, outputPath string) *exec.Cmd {
	return exec.CommandContext(
		ctx, b.javaPath, "-jar", b.decompileTool, "-mpm=30", artifactPath, outputPath)
}

type DecomplierResponse struct {
	Artifacts         []JavaArtifact
	ouputLocationBase string
	err               error
}

type internalDecompiler interface {
	internalDecompileIntoProject(context context.Context, binaryPath, projectPath string, responseChannel chan DecomplierResponse, waitGroup *sync.WaitGroup) error
	internalDecompile(context context.Context, binaryPath string, responseChannel chan DecomplierResponse, waitGroup *sync.WaitGroup) error
}
type Decompiler interface {
	DecompileIntoProject(context context.Context, binaryPath, projectPath string) ([]JavaArtifact, error)
	Decompile(context context.Context, binaryPath string) ([]JavaArtifact, error)
}

// The Decompiler will spin up some number of threads, and then a worker will take the job
// Executing the decompilation and file movement that must occur for the given job.
type decompiler struct {
	decompileTool     string
	log               logr.Logger
	workers           int
	labeler           labels.Labeler
	jobs              chan decompileJob
	cancelWorkersFunc context.CancelFunc
	java              string
	m2Repo            string
	mavenIndexPath    string
	// This should be set here when starting the decompiler
}

type DecompilerOpts struct {
	DecompileTool  string
	log            logr.Logger
	workers        int
	labler         labels.Labeler
	m2Repo         string
	mavenIndexPath string
}

func getDecompiler(options DecompilerOpts) (Decompiler, error) {
	log := options.log.WithName("decompiler")
	java := filepath.Join(os.Getenv("JAVA_HOME"), "bin", "java")
	d := decompiler{
		decompileTool:  options.DecompileTool,
		log:            log,
		workers:        options.workers,
		labeler:        options.labler,
		jobs:           make(chan decompileJob, 30),
		java:           java,
		m2Repo:         options.m2Repo,
		mavenIndexPath: options.mavenIndexPath,
	}
	// create and save decompile jobs channel.
	// Start Worker threads
	ctx, workerCacnelFunc := context.WithCancel(context.Background())
	for i := range options.workers {
		go d.decompileWorker(ctx, i)
	}
	d.cancelWorkersFunc = workerCacnelFunc
	// return DecompilerOpts
	return &d, nil
}

// Decompile will treat the artifact as a dependency, Trying to make an JavaArtifact from it
// To be handled with maven as a dependency.
func (d *decompiler) Decompile(ctx context.Context, artifactPath string) ([]JavaArtifact, error) {
	// For right now, the only thing that can be handled this way is a Jar file. If it is not a jar file
	// we should error out.
	if filepath.Ext(artifactPath) != JavaArchive {
		return nil, fmt.Errorf("unable to treat %s as a dependency", artifactPath)
	}

	responseChannel := make(chan DecomplierResponse)
	waitGroup := sync.WaitGroup{}
	// Create the job.
	job := jarArtifact{
		baseArtifact: baseArtifact{
			artifactPath:        artifactPath,
			m2Repo:              d.m2Repo,
			decompileTool:       d.decompileTool,
			javaPath:            d.java,
			labeler:             d.labeler,
			mavenIndexPath:      d.mavenIndexPath,
			decompiler:          d,
			decompilerResponses: responseChannel,
			decompilerWG:        &waitGroup,
		},
	}

	waitGroup.Add(1)
	// For the entry point job in the public methods, we will run the job, and wait for it to complete
	err := job.Run(ctx, d.log)
	d.log.Info("here")
	if err != nil {
		return nil, err
	}
	errs := []error{}
	artifacts := []JavaArtifact{}
	receiverCtx, cancelFunc := context.WithCancel(ctx)
	go func() {
		for {
			select {
			case resp := <-responseChannel:
				job.decompilerWG.Done()
				if resp.err != nil {
					errs = append(errs, resp.err)
				}
				artifacts = append(artifacts, resp.Artifacts...)
			case <-receiverCtx.Done():
				return
			}
		}
	}()

	waitGroup.Wait()
	cancelFunc()

	// TODO make this into a real error type.
	if len(errs) != 0 {
		return artifacts, errs[0]
	}
	return artifacts, nil
}

func (d *decompiler) DecompileIntoProject(ctx context.Context, artifactPath, projectPath string) ([]JavaArtifact, error) {
	var job decompileJob
	var responseChannel chan DecomplierResponse
	var waitGroup *sync.WaitGroup
	var err error
	d.log.Info(fmt.Sprintf("starting Decompile for: %s", artifactPath))
	switch filepath.Ext(artifactPath) {
	case JavaArchive, WebArchive, EnterpriseArchive:
		// Get Job
		d.log.Info(fmt.Sprintf("get Decompile job for: %s", artifactPath))
		job, responseChannel, waitGroup, err = d.getIntoProjectJob(artifactPath, projectPath)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unable to treat %s as a dependency", artifactPath)
	}

	errs := []error{}
	artifacts := []JavaArtifact{}
	receiverCtx, cancelFunc := context.WithCancel(ctx)
	go func() {
		for {
			select {
			case resp := <-responseChannel:
				// Anything coming back here, should be inside an internal calls
				// which should handle there own Done for the working group.
				if resp.err != nil {
					errs = append(errs, resp.err)
				}
				artifacts = append(artifacts, resp.Artifacts...)
			case <-receiverCtx.Done():
				return
			}
		}
	}()
	// For the entry point job in the public methods, we will run the job, and wait for it to complete
	waitGroup.Add(1)
	err = job.Run(ctx, d.log)
	if err != nil {
		cancelFunc()
		return nil, err
	}
	waitGroup.Wait()
	cancelFunc()

	if len(errs) > 0 {
		return artifacts, errs[0]
	}

	return artifacts, nil
}

func (d *decompiler) getIntoProjectJob(artifactPath, projectPath string) (decompileJob, chan DecomplierResponse, *sync.WaitGroup, error) {
	switch filepath.Ext(artifactPath) {
	case JavaArchive:
		d.log.V(7).Info(fmt.Sprintf("getting java archive job: %s", artifactPath))
		responseChannel := make(chan DecomplierResponse)
		waitGroup := sync.WaitGroup{}
		// Create the job.
		return &jarExplodeArtifact{
			explodeArtifact: explodeArtifact{
				baseArtifact: baseArtifact{
					artifactPath:        artifactPath,
					m2Repo:              d.m2Repo,
					decompileTool:       d.decompileTool,
					javaPath:            d.java,
					labeler:             d.labeler,
					mavenIndexPath:      d.mavenIndexPath,
					decompiler:          d,
					decompilerResponses: responseChannel,
					decompilerWG:        &waitGroup,
				},
				outputPath: projectPath,
			},
			tmpDir:         "",
			ctx:            nil,
			foundClassDirs: map[string]struct{}{},
		}, responseChannel, &waitGroup, nil
	case WebArchive:
		d.log.V(7).Info("getting web archive job")
		responseChannel := make(chan DecomplierResponse)
		waitGroup := sync.WaitGroup{}
		return &warArtifact{
			explodeArtifact: explodeArtifact{
				baseArtifact: baseArtifact{
					artifactPath:        artifactPath,
					m2Repo:              d.m2Repo,
					decompileTool:       d.decompileTool,
					javaPath:            d.java,
					labeler:             d.labeler,
					mavenIndexPath:      d.mavenIndexPath,
					decompiler:          d,
					decompilerResponses: responseChannel,
					decompilerWG:        &waitGroup,
				},
				outputPath: projectPath,
			},
			tmpDir: "",
			ctx:    nil,
		}, responseChannel, &waitGroup, nil
	case EnterpriseArchive:
		d.log.V(7).Info("getting enterprise archive job")
		responseChannel := make(chan DecomplierResponse)
		waitGroup := sync.WaitGroup{}
		return &earArtifact{
			explodeArtifact: explodeArtifact{
				baseArtifact: baseArtifact{
					artifactPath:        artifactPath,
					m2Repo:              d.m2Repo,
					decompileTool:       d.decompileTool,
					javaPath:            d.java,
					labeler:             d.labeler,
					mavenIndexPath:      d.mavenIndexPath,
					decompiler:          d,
					decompilerResponses: responseChannel,
					decompilerWG:        &waitGroup,
				},
				outputPath: projectPath,
			},
			tmpDir:       "",
			ctx:          nil,
			archiveFiles: []string{},
		}, responseChannel, &waitGroup, nil

	}
	return nil, nil, nil, fmt.Errorf("unable to get a job for the artifact")

}

// Internal Decompile calls will return with the number of jobs submitted to the queue
// The main Decompile jobs should be the only thing that waits based on the all the jobs
// that have been submitted.
func (d *decompiler) internalDecompile(ctx context.Context, artifactPath string, response chan DecomplierResponse, waitGroup *sync.WaitGroup) error {
	if filepath.Ext(artifactPath) != JavaArchive {
		return fmt.Errorf("unable to treat %s as a dependency", artifactPath)
	}
	job := jarArtifact{
		baseArtifact: baseArtifact{
			artifactPath:        artifactPath,
			m2Repo:              d.m2Repo,
			decompileTool:       d.decompileTool,
			javaPath:            d.java,
			labeler:             d.labeler,
			mavenIndexPath:      d.mavenIndexPath,
			decompiler:          d,
			decompilerResponses: response,
			decompilerWG:        waitGroup,
		},
	}
	waitGroup.Add(1)
	d.jobs <- &job
	return nil
}

func (d *decompiler) internalDecompileIntoProject(ctx context.Context, artifactPath, projectPath string, response chan DecomplierResponse, waitGroup *sync.WaitGroup) error {
	var job decompileJob
	var err error
	d.log.Info(fmt.Sprintf("starting Decompile for: %s", artifactPath))
	switch filepath.Ext(artifactPath) {
	case JavaArchive, WebArchive, EnterpriseArchive:
		// Get Job
		d.log.Info(fmt.Sprintf("get Decompile job for: %s", artifactPath))
		job, err = d.getIntoProjectJobInternal(artifactPath, projectPath, response, waitGroup)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unable to treat %s as a dependency", artifactPath)
	}

	// For the entry point job in the public methods, we will run the job, and wait for it to complete
	waitGroup.Add(1)
	d.jobs <- job
	return nil
}

func (d *decompiler) getIntoProjectJobInternal(artifactPath, projectPath string, responseChannel chan DecomplierResponse, waitGroup *sync.WaitGroup) (decompileJob, error) {
	switch filepath.Ext(artifactPath) {
	case JavaArchive:
		d.log.V(7).Info(fmt.Sprintf("getting java archive job: %s", artifactPath))
		// Create the job.
		return &jarExplodeArtifact{
			explodeArtifact: explodeArtifact{
				baseArtifact: baseArtifact{
					artifactPath:        artifactPath,
					m2Repo:              d.m2Repo,
					decompileTool:       d.decompileTool,
					javaPath:            d.java,
					labeler:             d.labeler,
					mavenIndexPath:      d.mavenIndexPath,
					decompiler:          d,
					decompilerResponses: responseChannel,
					decompilerWG:        waitGroup,
				},
				outputPath: projectPath,
			},
			tmpDir:         "",
			ctx:            nil,
			foundClassDirs: map[string]struct{}{},
		}, nil
	case WebArchive:
		d.log.V(7).Info("getting web archive job")
		return &warArtifact{
			explodeArtifact: explodeArtifact{
				baseArtifact: baseArtifact{
					artifactPath:        artifactPath,
					m2Repo:              d.m2Repo,
					decompileTool:       d.decompileTool,
					javaPath:            d.java,
					labeler:             d.labeler,
					mavenIndexPath:      d.mavenIndexPath,
					decompiler:          d,
					decompilerResponses: responseChannel,
					decompilerWG:        waitGroup,
				},
				outputPath: projectPath,
			},
			tmpDir: "",
			ctx:    nil,
		}, nil
	case EnterpriseArchive:
		d.log.V(7).Info("getting enterprise archive job")
		return &earArtifact{
			explodeArtifact: explodeArtifact{
				baseArtifact: baseArtifact{
					artifactPath:        artifactPath,
					m2Repo:              d.m2Repo,
					decompileTool:       d.decompileTool,
					javaPath:            d.java,
					labeler:             d.labeler,
					mavenIndexPath:      d.mavenIndexPath,
					decompiler:          d,
					decompilerResponses: responseChannel,
					decompilerWG:        waitGroup,
				},
				outputPath: projectPath,
			},
			tmpDir:       "",
			ctx:          nil,
			archiveFiles: []string{},
		}, nil

	}
	return nil, fmt.Errorf("unable to get a job fo rthe artifact")

}

func (d *decompiler) decompileWorker(ctx context.Context, workerID int) {
	log := d.log.WithValues("worker", workerID)
	for {
		select {
		case <-ctx.Done():
			log.Info("shutting down worker")
			return
		case decompileJob := <-d.jobs:
			err := decompileJob.Run(ctx, log)
			if err != nil {
				log.Error(err, "unable to decompile")
			}
		}
	}
}

func deduplicateJavaArtifacts(artifacts []JavaArtifact) []JavaArtifact {
	uniq := []JavaArtifact{}
	seen := map[string]bool{}
	for _, a := range artifacts {
		key := fmt.Sprintf("%s-%s-%s%s",
			a.ArtifactId, a.GroupId, a.Version, a.Packaging)
		if _, ok := seen[key]; !ok {
			seen[key] = true
			uniq = append(uniq, a)
		}
	}
	return uniq
}

func removeIncompleteDependencies(dependencies []JavaArtifact) []JavaArtifact {
	complete := []JavaArtifact{}
	for _, dep := range dependencies {
		if dep.ArtifactId != "" && dep.GroupId != "" && dep.Version != "" {
			complete = append(complete, dep)
		}
	}
	return complete
}
