package gradletasks

// Gradle task definitions embedded as Go constants.
// These tasks are injected into Gradle builds to support dependency resolution
// and source JAR downloads for the analyzer.
//
// All constants are exported so they can be used by both the bldtool and dependency packages.

const (
	// ResolveDepsTaskGradle resolves all dependencies to Gradle cache (Gradle 4-8)
	ResolveDepsTaskGradle = `/**
 * Dependency resolution task - ensures all binary JARs are downloaded to Gradle cache
 * Compatible with Gradle 4-8
 * This task resolves all main configurations to trigger JAR downloads without copying files
 */
task konveyorResolveDependencies {
    doLast {
        try {
            println "Starting konveyorResolveDependencies (Gradle 4-8)..."
            def totalResolved = 0
            def totalUnresolved = 0

            allprojects { project ->
                println "Resolving dependencies for project: ${project.name}"

                // Target main classpaths that contain the actual dependencies
                def targetConfigs = [
                        'compileClasspath',
                        'runtimeClasspath',
                        'implementation',
                        'api',
                        'compile',           // Legacy Gradle 2-3
                        'runtime',           // Legacy Gradle 2-3
                        'testCompileClasspath',
                        'testRuntimeClasspath',
                        'testImplementation'
                ].findAll { configName ->
                    def config = project.configurations.findByName(configName)
                    return config != null && config.canBeResolved
                }

                targetConfigs.each { configName ->
                    try {
                        def config = project.configurations.getByName(configName)
                        println "  Resolving configuration: ${configName}"

                        // Force resolution - this triggers JAR downloads to Gradle cache
                        def resolvedConfig = config.resolvedConfiguration

                        // Report unresolved dependencies
                        if (resolvedConfig.hasError()) {
                            resolvedConfig.lenientConfiguration.unresolvedModuleDependencies.each { dep ->
                                println "    WARNING: Could not resolve: ${dep.selector} - ${dep.problem.message}"
                                totalUnresolved++
                            }
                        }

                        // Process successfully resolved artifacts
                        resolvedConfig.lenientConfiguration.artifacts.each { artifact ->
                            // Just accessing the file property ensures it's downloaded
                            if (artifact.file.exists()) {
                                totalResolved++
                            }
                        }

                        println "    Resolved ${resolvedConfig.lenientConfiguration.artifacts.size()} artifacts"
                    } catch (Exception e) {
                        println "    Warning: Could not fully resolve ${configName}: ${e.message}"
                        // Continue with other configurations
                    }
                }
            }

            println "konveyorResolveDependencies completed: ${totalResolved} artifacts resolved, ${totalUnresolved} unresolved"
            if (totalUnresolved > 0) {
                println "WARNING: Some dependencies could not be resolved. This may affect analysis completeness."
            }
        } catch (Exception e) {
            println "konveyorResolveDependencies encountered an error: ${e.message}"
            e.printStackTrace()
            // Don't fail the build - continue even if some dependencies couldn't be resolved
        }
    }
}
`

	// ResolveDepsTaskGradle9 resolves all dependencies to Gradle cache (Gradle 9+)
	ResolveDepsTaskGradle9 = `/**
 * Configuration cache compatible dependency resolution task - compatible with Gradle 8.14+
 * Ensures all binary JARs are downloaded to Gradle cache
 * All project iteration happens at configuration time
 */

// Track resolved artifacts count at configuration time
def resolvedArtifactsCount = 0
def unresolvedArtifactsCount = 0

println "konveyorResolveDependencies (v9): configuration phase starting..."
allprojects { proj ->
    try {
        // Process each project during configuration phase
        def targetConfigs = [
            'compileClasspath',
            'runtimeClasspath',
            'implementation',
            'api',
            'testCompileClasspath',
            'testRuntimeClasspath',
            'testImplementation'
        ].findAll { configName ->
            def config = proj.configurations.findByName(configName)
            return config != null && config.canBeResolved
        }

        targetConfigs.each { configName ->
            try {
                def config = proj.configurations.getByName(configName)

                // Use modern incoming artifacts API with lenient resolution
                def artifactView = config.incoming.artifactView { view ->
                    view.lenient(true)
                }

                def result = artifactView.artifacts

                // Report resolution failures
                result.failures.each { failure ->
                    println "    WARNING: Could not resolve artifact in ${proj.name}:${configName}: ${failure.message}"
                    unresolvedArtifactsCount++
                }

                // Accessing the artifacts triggers resolution and download
                result.artifacts.each { artifactResult ->
                    try {
                        // Accessing the file property ensures it's downloaded to cache
                        def file = artifactResult.file
                        if (file != null && file.exists()) {
                            resolvedArtifactsCount++
                        }
                    } catch (Exception e) {
                        // Some artifacts may fail - continue with others
                        println "    Warning: Could not resolve artifact in ${proj.name}:${configName}: ${e.message}"
                        unresolvedArtifactsCount++
                    }
                }

                println "Resolved ${result.artifacts.size()} artifacts for ${proj.name}:${configName}"
            } catch (Exception e) {
                println "Warning: Could not process ${proj.name}:${configName}: ${e.message}"
                // Continue with other configurations
            }
        }
    } catch (Exception e) {
        println "konveyorResolveDependencies (v9): Warning for project ${proj.name}: ${e.message}"
        // Continue with other projects
    }
}
println "konveyorResolveDependencies (v9): configuration phase finished, ${resolvedArtifactsCount} artifacts resolved, ${unresolvedArtifactsCount} unresolved"

task konveyorResolveDependencies {
    // Store the counts for reporting
    def artifactsCount = resolvedArtifactsCount
    def unresolvedCount = unresolvedArtifactsCount

    doLast {
        println "konveyorResolveDependencies (Gradle 9+) execution phase complete"
        println "Total artifacts resolved: ${artifactsCount}"
        if (unresolvedCount > 0) {
            println "WARNING: ${unresolvedCount} dependencies could not be resolved. This may affect analysis completeness."
        }
    }
}
`

	// DownloadSourcesTaskGradle downloads source JARs where available (Gradle 4-8)
	DownloadSourcesTaskGradle = `/**
 * Sources download task - downloads source JARs and reports which ones are missing
 * Compatible with Gradle 4-8
 */
task konveyorDownloadSources {
    doLast {
        try {
            println "Starting konveyorDownloadSources (Gradle 4-8)..."
            def allSourceFiles = []

            allprojects { project ->
                println "Processing project: ${project.name}"

                // Focus on main classpaths
                def targetConfigs = [
                        'compileClasspath',
                        'runtimeClasspath',
                        'implementation',
                        'api'
                ].findAll { configName ->
                    project.configurations.findByName(configName)?.canBeResolved ?: false
                }

                targetConfigs.each { configName ->
                    try {
                        def config = project.configurations.getByName(configName)
                        println "  Processing configuration: ${configName}"

                        // Get resolved dependencies
                        def resolvedConfig = config.resolvedConfiguration
                        def dependencies = resolvedConfig.resolvedArtifacts

                        // Extract module identifiers for source resolution
                        def moduleIds = dependencies.collect { artifact ->
                            artifact.moduleVersion.id
                        }.unique()

                        if (!moduleIds.isEmpty()) {
                            println "    Found ${moduleIds.size()} unique modules"

                            // Query for sources using the dependency notation
                            moduleIds.each { moduleId ->
                                def sourceDep = null
                                def sourceConfig = null
                                def sourceFiles = []
                                try {
                                    sourceDep = project.dependencies.create(
                                            group: moduleId.group,
                                            name: moduleId.name,
                                            version: moduleId.version,
                                            classifier: 'sources'
                                    )

                                    sourceConfig = project.configurations.detachedConfiguration(sourceDep)
                                    sourceConfig.transitive = false
                                    sourceFiles = sourceConfig.resolve()

                                    if (!sourceFiles.isEmpty()) {
                                        allSourceFiles.addAll(sourceFiles)
                                        println "Found 1 sources for ${moduleId.group}:${moduleId.name}:${moduleId.version}"
                                    } else {
                                        println "Found 0 sources for ${moduleId.group}:${moduleId.name}:${moduleId.version}"
                                    }
                                } catch (Exception e) {
                                    println "Found 0 sources for ${moduleId.group}:${moduleId.name}:${moduleId.version}"
                                }
                            }
                        }
                    } catch (Exception e) {
                        println "    Error processing ${configName}: ${e.message}"
                    }
                }
            }

            // Copy all found source files to build/downloads
            if (!allSourceFiles.isEmpty()) {
                try {
                    def downloadDir = new File(project.buildDir, "downloads")
                    if (!downloadDir.exists()) {
                        downloadDir.mkdirs()
                    }
                    copy {
                        from allSourceFiles
                        into downloadDir
                        duplicatesStrategy = "exclude"
                    }
                    println "Downloaded ${allSourceFiles.size()} source files to ${downloadDir}"
                } catch (Exception e) {
                    println "ERROR copying source files: ${e.message}"
                    e.printStackTrace()
                }
            } else {
                println "No source files found to download"
            }
        } catch (Exception e) {
            println "konveyorDownloadSources FAILED: ${e.message}"
            e.printStackTrace()
            throw e
        }
    }
}
`

	// DownloadSourcesTaskGradle9 downloads source JARs where available (Gradle 9+)
	DownloadSourcesTaskGradle9 = `/**
 * Configuration cache compatible sources download task - compatible with Gradle 8.14+
 * All project iteration happens at configuration time
 */

// Collect all source files at configuration time
def allProjectSourceFiles = []
def sourceResolutionResults = []

println "konveyorDownloadSources (v9): configuration phase starting..."
allprojects { proj ->
    try {
        // Process each project during configuration phase
        def targetConfigs = [
            'compileClasspath',
            'runtimeClasspath',
            'implementation',
            'api'
        ].findAll { configName ->
            def config = proj.configurations.findByName(configName)
            return config != null && config.canBeResolved
        }

        targetConfigs.each { configName ->
            try {
                def config = proj.configurations.getByName(configName)

                // Use modern incoming artifacts API
                def artifacts = config.incoming.artifacts
                def artifactResults = artifacts.artifacts

                // Extract module identifiers for source resolution
                def moduleIds = artifactResults.collect { artifactResult ->
                    def componentId = artifactResult.id.componentIdentifier
                    if (componentId instanceof ModuleComponentIdentifier) {
                        return [
                            group: componentId.group,
                            name: componentId.module,
                            version: componentId.version
                        ]
                    }
                    return null
                }.findAll { it != null }.unique()

                if (!moduleIds.isEmpty()) {
                    // Try to resolve sources for each module
                    moduleIds.each { moduleId ->
                        try {
                            def sourceDep = proj.dependencies.create(
                                "${moduleId.group}:${moduleId.name}:${moduleId.version}:sources"
                            )

                            def sourcesConfig = proj.configurations.detachedConfiguration(sourceDep)
                            sourcesConfig.transitive = false

                            def sourceFiles = sourcesConfig.incoming.artifactView { view ->
                                view.lenient(true)
                            }.artifacts.artifacts.collect { it.file }.findAll {
                                it != null && it.exists()
                            }

                            if (!sourceFiles.isEmpty()) {
                                allProjectSourceFiles.addAll(sourceFiles)
                                sourceResolutionResults.add("Found 1 sources for ${moduleId.group}:${moduleId.name}:${moduleId.version}")
                            } else {
                                sourceResolutionResults.add("Found 0 sources for ${moduleId.group}:${moduleId.name}:${moduleId.version}")
                            }
                        } catch (Exception e) {
                            sourceResolutionResults.add("Found 0 sources for ${moduleId.group}:${moduleId.name}:${moduleId.version}")
                        }
                    }
                }
            } catch (Exception e) {
                println "Error processing ${proj.name}:${configName}: ${e.message}"
            }
        }
    } catch (Exception e) {
        println "konveyorDownloadSources (v9): WARNING for project ${proj.name}: ${e.message}"
    }
}
println "konveyorDownloadSources (v9): configuration phase finished, ${allProjectSourceFiles.size()} source files collected"

task konveyorDownloadSources {
    // Store the collected files as task input
    def sourceFiles = allProjectSourceFiles
    def results = sourceResolutionResults

    doLast {
        try {
            println "Starting konveyorDownloadSources (Gradle 9+) execution phase..."

            // Print all resolution results
            results.each { result ->
                println result
            }

            if (!sourceFiles.isEmpty()) {
                try {
                    def downloadDir = new File(project.layout.buildDirectory.get().asFile, "downloads")
                    if (!downloadDir.exists()) {
                        downloadDir.mkdirs()
                    }
                    copy {
                        from sourceFiles
                        into downloadDir
                        duplicatesStrategy = DuplicatesStrategy.EXCLUDE
                    }
                    println "Downloaded ${sourceFiles.size()} source files to ${downloadDir}"
                } catch (Exception e) {
                    println "ERROR copying source files: ${e.message}"
                    e.printStackTrace()
                }
            } else {
                println "No source files found to download"
            }
        } catch (Exception e) {
            println "konveyorDownloadSources FAILED: ${e.message}"
            e.printStackTrace()
            throw e
        }
    }
}
`
)
