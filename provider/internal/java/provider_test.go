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
The following files have been resolved:
   org.springframework.boot:spring-boot:jar:sources:2.5.0:compile

The following files have NOT been resolved:
   io.konveyor.demo:config-utils:jar:sources:1.0.0:compile
`,
			wantErr: false,
			wantList: []javaArtifact{
				{
					packaging:  JavaArchive,
					groupId:    "io.konveyor.demo",
					artifactId: "config-utils",
					version:    "1.0.0",
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
