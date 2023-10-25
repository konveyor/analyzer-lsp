package java

import (
	"reflect"
	"strings"
	"testing"
)

func Test_parseUnresolvedSources(t *testing.T) {
	tests := []struct {
		name      string
		mvnOutput string
		wantErr   bool
		wantList  []javaArtifact
	}{
		{
			name: "valid sources output",
			mvnOutput: `
[INFO] --- maven-dependency-plugin:3.5.0:sources (default-cli) @ spring-petclinic ---
[INFO] The following files have been resolved:
[INFO]    org.springframework.boot:spring-boot-starter-actuator:jar:sources:3.1.0 -- module spring.boot.starter.actuator [auto]
[INFO]    org.springframework.boot:spring-boot-starter:jar:sources:3.1.0 -- module spring.boot.starter [auto]
[INFO]    org.springframework.boot:spring-boot-starter-logging:jar:sources:3.1.0 -- module spring.boot.starter.logging [auto]
[INFO] The following files have NOT been resolved:
[INFO]    org.springframework.boot:spring-boot-starter-logging:jar:3.1.0:compile -- module spring.boot.starter.logging [auto]
[INFO]    io.konveyor.demo:config-utils:jar:sources:1.0.0:compile
[INFO] --- maven-dependency-plugin:3.5.0:sources (default-cli) @ spring-petclinic ---
[INFO] -----------------------------------------------------------------------------
[INFO] The following files have NOT been resolved:
[INFO]    org.springframework.boot:spring-boot-actuator:jar:sources:3.1.0:compile
`,
			wantErr: false,
			wantList: []javaArtifact{
				{
					packaging:  JavaArchive,
					GroupId:    "io.konveyor.demo",
					ArtifactId: "config-utils",
					Version:    "1.0.0",
				},
				{
					packaging:  JavaArchive,
					GroupId:    "org.springframework.boot",
					ArtifactId: "spring-boot-actuator",
					Version:    "3.1.0",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outputReader := strings.NewReader(tt.mvnOutput)
			gotList, gotErr := parseUnresolvedSources(outputReader)
			if (gotErr != nil) != tt.wantErr {
				t.Errorf("parseUnresolvedSources() gotErr = %v, wantErr %v", gotErr, tt.wantErr)
			}
			if !reflect.DeepEqual(gotList, tt.wantList) {
				t.Errorf("parseUnresolvedSources() gotList = %v, wantList %v", gotList, tt.wantList)
			}
		})
	}
}
