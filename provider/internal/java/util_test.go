package java

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestRenderPom(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Define some sample dependencies
	dependencies := []javaArtifact{
		{
			GroupId:    "com.example",
			ArtifactId: "example-artifact",
			Version:    "1.0.0",
		},
		{
			GroupId:    "org.another",
			ArtifactId: "another-artifact",
			Version:    "2.0.0",
		},
	}

	// Call the function with the temporary directory and sample dependencies
	err := createJavaProject(nil, tmpDir, dependencies)
	if err != nil {
		t.Fatalf("createJavaProject returned an error: %v", err)
	}

	// Verify that the project directory and pom.xml file were created
	projectDir := filepath.Join(tmpDir, "src", "main", "java")
	pomFile := filepath.Join(tmpDir, "pom.xml")

	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		t.Errorf("Java source directory not created: %v", err)
	}

	if _, err := os.Stat(pomFile); os.IsNotExist(err) {
		t.Errorf("pom.xml file not created: %v", err)
	}

	// Read the generated pom.xml content
	pomContent, err := os.ReadFile(pomFile)
	if err != nil {
		t.Fatalf("error reading pom.xml file: %v", err)
	}

	// Define the expected pom.xml content
	expectedPom := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
  xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 http://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>

  <groupId>io.konveyor</groupId>
  <artifactId>java-project</artifactId>
  <version>1.0-SNAPSHOT</version>

  <name>java-project</name>
  <url>http://www.konveyor.io</url>

  <properties>
    <project.build.sourceEncoding>UTF-8</project.build.sourceEncoding>
  </properties>

  <dependencies>

    <dependency>
      <groupId>com.example</groupId>
      <artifactId>example-artifact</artifactId>
      <version>1.0.0</version>
    </dependency>

    <dependency>
      <groupId>org.another</groupId>
      <artifactId>another-artifact</artifactId>
      <version>2.0.0</version>
    </dependency>

  </dependencies>

  <build>
  </build>
</project>
`

	// Compare the generated pom.xml content with the expected content
	if !bytes.Equal(pomContent, []byte(expectedPom)) {
		t.Errorf("Generated pom.xml content does not match the expected content")
		fmt.Println(string(pomContent))
		fmt.Println("expected POM")
		fmt.Println(expectedPom)
	}
}
