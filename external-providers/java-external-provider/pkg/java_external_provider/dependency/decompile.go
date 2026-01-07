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

// decompileFilter determines whether a specific JavaArtifact should be decompiled.
// Different implementations can provide filtering logic based on artifact properties.
type decompileFilter interface {
	shouldDecompile(JavaArtifact) bool
}

// alwaysDecompileFilter is a simple boolean filter that always returns the same decision.
// When true, all artifacts will be decompiled. When false, none will be.
type alwaysDecompileFilter bool

func (a alwaysDecompileFilter) shouldDecompile(j JavaArtifact) bool {
	return bool(a)
}

// decompileJob represents a unit of work for the decompiler worker pool.
// Each job is responsible for decompiling a specific artifact (JAR, WAR, or EAR)
// and signaling completion through the Done() method.
type decompileJob interface {
	Run(ctx context.Context, log logr.Logger) error
}

// baseArtifact provides common functionality for all artifact types being decompiled.
// It contains shared configuration and helper methods used by jarArtifact, warArtifact,
// earArtifact, and jarExplodeArtifact implementations.
type baseArtifact struct {
	artifactPath        string                  // Absolute path to the artifact file being decompiled
	m2Repo              string                  // Path to Maven local repository for storing decompiled artifacts
	decompileTool       string                  // Absolute path to the FernFlower decompiler JAR
	javaPath            string                  // Path to java executable for running decompiler
	labeler             labels.Labeler          // Labeler for classifying dependencies
	mavenIndexPath      string                  // Path to Maven index for artifact lookups
	decompiler          internalDecompiler      // Reference to decompiler for nested artifact processing
	decompilerResponses chan DecomplierResponse // Channel for receiving decompilation results
	decompilerWG        *sync.WaitGroup         // WaitGroup for coordinating job completion
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

// DecomplierResponse contains the results from a decompilation operation.
// It is sent through a channel to communicate results from worker goroutines.
type DecomplierResponse struct {
	Artifacts         []JavaArtifact // List of artifacts discovered during decompilation
	ouputLocationBase string         // Base directory where decompiled output was written
	err               error          // Error if decompilation failed
}

// internalDecompiler is an internal interface for recursive decompilation operations.
// It's used by artifact jobs to trigger decompilation of nested artifacts (e.g., JARs within WARs).
type internalDecompiler interface {
	internalDecompileIntoProject(context context.Context, binaryPath, projectPath string, responseChannel chan DecomplierResponse, waitGroup *sync.WaitGroup) error
	internalDecompile(context context.Context, binaryPath string, responseChannel chan DecomplierResponse, waitGroup *sync.WaitGroup) error
	internalDecompileClasses(context context.Context, classDirectory, output string, responseChannel chan DecomplierResponse, waitGroup *sync.WaitGroup) error
}

// Decompiler is the public interface for decompiling Java binary artifacts.
// It provides two modes of operation:
//   - Decompile: Treats artifact as a dependency, creating Maven repository structure
//   - DecompileIntoProject: Decompiles into a project directory for analysis
//
// The decompiler uses a worker pool to parallelize decompilation of multiple artifacts.
type Decompiler interface {
	// DecompileIntoProject decompiles a binary artifact into a project directory structure.
	// Used for decompiling application binaries (not dependencies).
	//
	// Returns list of discovered JavaArtifacts from embedded dependencies.
	DecompileIntoProject(context context.Context, binaryPath, projectPath string) ([]JavaArtifact, error)

	// Decompile treats an artifact as a dependency and decompiles it into Maven repository structure.
	// Creates proper groupId/artifactId/version directory hierarchy in the local Maven repository.
	//
	// Returns list of JavaArtifacts including the main artifact and any discovered embedded dependencies.
	Decompile(context context.Context, binaryPath string) ([]JavaArtifact, error)
}

// decompiler implements the Decompiler interface using a worker pool pattern.
// It spawns multiple worker goroutines that process decompilation jobs concurrently,
// significantly improving performance when decompiling multiple artifacts.
//
// Worker Pool Architecture:
//   - Configurable number of workers (default: 10)
//   - Job queue (channel) for distributing work
//   - Supports JAR, WAR, and EAR files
//   - Recursive decompilation of nested archives
type decompiler struct {
	decompileTool     string             // Path to FernFlower decompiler JAR
	log               logr.Logger        // Logger for decompiler operations
	workers           int                // Number of worker goroutines in the pool
	labeler           labels.Labeler     // Labeler for dependency classification
	jobs              chan decompileJob  // Channel for distributing decompilation jobs to workers
	cancelWorkersFunc context.CancelFunc // Function to cancel all worker goroutines
	java              string             // Path to java executable
	m2Repo            string             // Path to Maven local repository
	mavenIndexPath    string             // Path to Maven index for artifact lookups
}

// DecompilerOpts contains configuration options for creating a Decompiler instance.
// All fields must be properly initialized except workers which defaults to DefaultWorkerPoolSize if zero.
type DecompilerOpts struct {
	DecompileTool  string         // Absolute path to FernFlower decompiler JAR
	log            logr.Logger    // Logger instance for decompiler operations
	workers        int            // Number of worker goroutines (0 = use DefaultWorkerPoolSize)
	labler         labels.Labeler // Labeler for classifying dependencies as open-source or internal
	m2Repo         string         // Path to Maven local repository for storing decompiled artifacts
	mavenIndexPath string         // Path to Maven index directory for artifact lookups
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
	if d.workers == 0 {
		d.workers = DefaultWorkerPoolSize
	}
	// create and save decompile jobs channel.
	// Start Worker threads
	ctx, workerCacnelFunc := context.WithCancel(context.Background())
	for i := range d.workers {
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
	errs := []error{}
	artifacts := []JavaArtifact{}
	receiverCtx, cancelFunc := context.WithCancel(ctx)
	go func() {
		for {
			select {
			case resp := <-responseChannel:
				waitGroup.Done()
				if resp.err != nil {
					errs = append(errs, resp.err)
				}
				artifacts = append(artifacts, resp.Artifacts...)
			case <-receiverCtx.Done():
				return
			}
		}
	}()

	d.log.V(9).Info("adding", "artifact", artifactPath)
	waitGroup.Add(1)
	// For the entry point job in the public methods, we will run the job, and wait for it to complete
	err := job.Run(ctx, d.log)
	if err != nil {
		cancelFunc()
		return nil, err
	}
	waitGroup.Wait()
	cancelFunc()
	d.log.Info("completed decompile", "artifact", artifactPath)

	// TODO make this into a real error type.
	if len(errs) != 0 {
		return artifacts, errs[0]
	}
	return artifacts, nil
}

func (d *decompiler) DecompileIntoProject(ctx context.Context, artifactPath, projectPath string) ([]JavaArtifact, error) {
	var job decompileJob
	responseChannel := make(chan DecomplierResponse)
	waitGroup := sync.WaitGroup{}
	var err error
	d.log.Info(fmt.Sprintf("starting Decompile for: %s", artifactPath))
	switch filepath.Ext(artifactPath) {
	case JavaArchive, WebArchive, EnterpriseArchive:
		// Get Job
		d.log.Info(fmt.Sprintf("get Decompile job for: %s", artifactPath))
		job, err = d.getIntoProjectJob(artifactPath, projectPath, responseChannel, &waitGroup)
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
				d.log.Info("got response", "response", resp, "wg", fmt.Sprintf("%#v", &waitGroup))
				waitGroup.Done()
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
	d.log.V(9).Info("adding", "artifact", artifactPath)
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
	d.log.V(9).Info("adding", "artifact", artifactPath)
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
		job, err = d.getIntoProjectJob(artifactPath, projectPath, response, waitGroup)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unable to treat %s as a dependency", artifactPath)
	}

	// For the entry point job in the public methods, we will run the job, and wait for it to complete
	d.log.V(9).Info("adding", "artifact", artifactPath)
	waitGroup.Add(1)
	d.jobs <- job
	return nil
}

func (d *decompiler) getIntoProjectJob(artifactPath, projectPath string, responseChannel chan DecomplierResponse, waitGroup *sync.WaitGroup) (decompileJob, error) {
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

func (d *decompiler) internalDecompileClasses(ctx context.Context, classDirPath, output string, responseChan chan DecomplierResponse, waitGroup *sync.WaitGroup) error {
	d.log.V(9).Info("adding", "artifact", classDirPath)
	waitGroup.Add(1)
	d.jobs <- &classDecompileJob{
		classDirPath:     classDirPath,
		outputPath:       output,
		decompileTool:    d.decompileTool,
		responseChanndel: responseChan,
		wg:               waitGroup,
		javaPath:         d.java,
		log:              logr.Logger{},
	}
	return nil
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
