package generic

import "strings"

// Ideally we wouldn't have to do anything like this. Alas, we must.

// 2-tiered table for each language and their respective fixes we may need to
// apply
var SubstitutionTable = map[string]map[string]string{
	"python": {
		"{\"cells\":[{\"language\":\"python\"}]}": "[{\"cells\":[{\"language\":\"python\"}]}]",
	},
}

// Uses SubstitutionTable to replace every occurance of the offending part. Not
// very efficent, may have to investigate alternatives. Fails silently.
func NaiveFixResponse(language string, resp string) string {
	m, ok := SubstitutionTable[language]
	if !ok {
		return resp
	}

	// O(n^2) at least
	for k, v := range m {
		resp = strings.Replace(resp, k, v, -1)
	}

	return resp
}
