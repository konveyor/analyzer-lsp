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
[INFO] Downloaded from central: https://repo.maven.apache.org/maven2/com/vladsch/flexmark/flexmark-util/0.42.14/flexmark-util-0.42.14.jar (385 kB at 301 kB/s)
[INFO] Downloaded from central: https://repo.maven.apache.org/maven2/javax/enterprise/cdi-api/1.2/cdi-api-1.2.jar (71 kB at 56 kB/s)
[INFO] Downloaded from central: https://repo.maven.apache.org/maven2/org/apache/httpcomponents/httpcore/4.4.14/httpcore-4.4.14.jar (328 kB at 253 kB/s)
[WARNING] The following artifacts could not be resolved: antlr:antlr:jar:sources:2.7.7 (absent), io.konveyor.demo:config-utils:jar:1.0.0 (absent), io.konveyor.demo:config-utils:jar:sources:1.0.0 (absent): Could not find artifact antlr:antlr:jar:sources:2.7.7 in central (https://repo.maven.apache.org/maven2)
[INFO] ------------------------------------------------------------------------
[INFO] BUILD SUCCESS
[INFO] ------------------------------------------------------------------------
[INFO] Total time:  16.485 s
[INFO] Finished at: 2023-11-15T12:52:59Z
[INFO] ------------------------------------------------------------------------
`,
			wantErr: false,
			wantList: []javaArtifact{
				{
					packaging:  JavaArchive,
					GroupId:    "antlr",
					ArtifactId: "antlr",
					Version:    "2.7.7",
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
