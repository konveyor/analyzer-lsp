# Analyzer and Provider Interaction

The analyzer works in tandem with the provider to effectively analyze and transform application code. Here's a detailed explanation of how this interaction typically works:

## Provider vs Rulesets

A **ruleset** is essentially a collection of rules that are used to analyze and transform code from one language to another or to adapt it to a different environment. Rulesets define the specific transformations, checks, and validations needed to handle different programming languages or frameworks.

A **provider** refers to the entity or service that supplies these rulesets. Providers are tools that maintain and distribute rulesets for various languages and environments. They are responsible for ensuring that the rulesets are up-to-date, comprehensive, and accurate. Currently, the supported providers are Java, .NET, Go, Python, and Node.js. Java is fully supported, while .NET has a custom rule to assist with project analysis. Go, Python, and Node.js also require custom rules to function properly.

## Interaction Between the Analyzer and Provider

### Initialization

When you run the `mta-cli` tool, it first determines the type of application and the corresponding provider needed (e.g., Java provider for a Java application).

### Starting the Provider

The `mta-cli` tool starts the provider in a container using a containerization tool like Docker or Podman. This containerized environment includes all necessary dependencies and tools required by the provider.

### Code Submission

The application’s source code or relevant parts of it are sent to the provider running in the container. This can be done by mounting volumes, copying files, or using APIs to transfer the code.

### Execution of Rulesets

The provider, now with access to the code, uses the analyzer to execute a series of rulesets. These rulesets are predefined sets of conditional logic checks on how to analyze the code, identify issues, suggest improvements, and possibly transform the code.

## Detailed Workflow

### Discovery and Analysis Initiation

- `mta-cli` identifies the language and type of the application.
- `mta-cli` initiates the corresponding provider in a container.

### Provider Setup

- The container starts, providing an isolated environment.
- The provider within the container has the analyzer tool and necessary libraries pre-installed.

### Analyzer Execution

- The analyzer tool within the provider starts processing the code.
- The analyzer reads the code, parses it into an Abstract Syntax Tree (AST), and applies the rulesets.

### Applying Rulesets

- The provider applies the rulesets to the source code using the analyzer.

### Generating Reports

- The analyzer compiles a report based on the analysis, detailing issues found, suggestions for improvement, and any transformations made.
- This report is a single static HTML file; in the hub, the data is served dynamically to be introspective further.

### Returning Results

- The running provider mounts common directories that are used for input/output which in return contain the analysis report.
- `mta-cli` presents the results to the user in a readable format.
