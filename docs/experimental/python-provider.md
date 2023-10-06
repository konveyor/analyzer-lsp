# Python Provider using Generic Provider

Updated: 2023-09-28

We are using [python-lsp-server](https://github.com/python-lsp/python-lsp-server) (`pylsp`) to create a Python provider using generic-external-provider.

**If you're interested in jumping to the results, [click here](#results).**

pylsp can be installed using

```sh
pip install python-lsp-server
```

It will be installed in `/home/<user_name>/.local/bin/pylsp`

The server can be run with the following optional arguments:

- `--log-file <file>`: Logs the output to a file
- `-v`: Info level logging
- `-v -v`: Debug level logging 

## Testing

### Config

```json
{
  "name": "python",
  "binaryPath": "/path/to/generic-external-provider",
  "initConfig": [{
    "location": "examples/python",
    "analysisMode": "full",
    "providerSpecificConfig": {
      "name": "python",
      "lspServerPath": "/path/to/pylsp",
      "lspArgs": ["--log-file", "debug-python.log"],
      "referencedOutputIgnoreContains": [
        "examples/python/__pycache__",
        "examples/python/.venv"
      ]
    }
  }]
},
```

### Example Project

We created a new virtual environment with `python3 -m venv .venv` and installed the kubernetes library with `pip install kubernetes`.

Thus, the file tree looked like this:

```
├── file_a.py
├── file_b.py
├── main.py
├── __pycache__
│   └── <misc files>
├── .venv
│   └── <misc files>
└── requirements.txt
```

To test simple cross-file references, we created file_a.py and file_b.py as follows:

```python
# file_a.py
import file_b

print(file_b.hello_world())

doggie = file_b.Dog()
print(doggie.speak())
```

```python
# file_b.py
def hello_world():
  return "Hello, world!"

class Dog(object):
  def __init__(self) -> None:
    pass
  
  def speak(self):
    return "Woof!"
```

For non-local, installed libraries we created main.py as follows:

```python
# main.py
# Source: https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/ApiextensionsV1Api.md

from __future__ import print_function
import time
import kubernetes.client
from kubernetes.client.rest import ApiException
from pprint import pprint
configuration = kubernetes.client.Configuration()
# Configure API key authorization: BearerToken
configuration.api_key['authorization'] = 'YOUR_API_KEY'
# Uncomment below to setup prefix (e.g. Bearer) for API key, if needed
# configuration.api_key_prefix['authorization'] = 'Bearer'

# Defining host is optional and default to http://localhost
configuration.host = "http://localhost"

# Enter a context with an instance of the API kubernetes.client
with kubernetes.client.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = kubernetes.client.ApiextensionsV1Api(api_client)
    body = kubernetes.client.V1CustomResourceDefinition() # V1CustomResourceDefinition | 
    pretty = 'pretty_example' # str | If 'true', then the output is pretty printed. (optional)
    dry_run = 'dry_run_example' # str | When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processed (optional)
    field_manager = 'field_manager_example' # str | fieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint. (optional)
    field_validation = 'field_validation_example' # str | fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered. (optional)

    try:
        api_response = api_instance.create_custom_resource_definition(body, pretty=pretty, dry_run=dry_run, field_manager=field_manager, field_validation=field_validation)
        pprint(api_response)
    except ApiException as e:
        print("Exception when calling ApiextensionsV1Api->create_custom_resource_definition: %s\n" % e)
```

### Rule

```yaml
- message: python sample rule 001
  ruleID: python-sample-rule-001
  when:
    python.referenced: 
      pattern: "hello_world"
- message: python sample rule 002
  ruleID: python-sample-rule-002
  when:
    python.referenced: 
      pattern: "speak"
- message: python sample rule 003
  ruleID: python-sample-rule-003
  when:
    python.referenced: 
      pattern: "create_custom_resource_definition"
```

## Findings

python-lsp-server returned the following response. Note that the line numbers are 0-indexed per the LSP spec.

```yaml
- name: konveyor-analysis
  violations:
    python-sample-rule-001:
      description: ""
      category: potential
      incidents:
      - uri: file:///path/to/python/file_a.py
        message: python sample rule 001
        lineNumber: 2
        variables:
          file: file:///path/to/python/file_a.py
      - uri: file:///path/to/python/file_b.py
        message: python sample rule 001
        lineNumber: 0
        variables:
          file: file:///path/to/python/file_b.py
    python-sample-rule-002:
      description: ""
      category: potential
      incidents:
      - uri: file:///path/to/python/file_a.py
        message: python sample rule 002
        lineNumber: 5
        variables:
          file: file:///path/to/python/file_a.py
      - uri: file:///path/to/python/file_b.py
        message: python sample rule 002
        lineNumber: 7
        variables:
          file: file:///path/to/python/file_b.py
    python-sample-rule-003:
      description: ""
      category: potential
      incidents:
      - uri: file:///path/to/python/main.py
        message: python sample rule 003
        lineNumber: 25
        variables:
          file: file:///path/to/python/main.py
```

## Results

As far as I am aware, this finishes the preliminary work on getting `referenced` to work with the python lsp server. 🥳

Unfortunately, there are still some wrinkles that need to be solved.

### Pylsp capabilities response incorrect

When responding with its capabilites, python-lsp-server returns an object. However, the spec requires it to be an array of objects. See [tables.go](../../external-providers/generic-external-provider/pkg/generic/tables.go) for the fix.

- https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#notebookDocumentSyncOptions
- https://github.com/python-lsp/python-lsp-server/blob/05698fa11bfc566ae7e040a2ed272247f8d406b2/pylsp/python_lsp.py#L298

### Pylsp includes definitions in results

`pylsp` is currently returning references that are usages *and* definitions. `gopls` does not do this. Theoretically, the code I added in `external-providers/generic-external-provider/pkg/generic/service_client.go` should fix this by setting IncludeDeclaration to false, but it doesn't for some reason:

```go
params := &protocol.ReferenceParams{
  TextDocumentPositionParams: protocol.TextDocumentPositionParams{
    TextDocument: protocol.TextDocumentIdentifier{
      URI: location.URI,
    },
    Position: location.Range.Start,
  },
  Context: protocol.ReferenceContext{
    // pylsp has trouble with always returning declarations
    IncludeDeclaration: false,
  },
}
```

### Gopls and Pylsp naming conventions

`gopls` and `pylsp` follow different naming conventions when searching for symbols. Take a look at the [SymbolInformation return value in the LSP spec](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#symbolInformation). Say there is a function called `frobinate` in a package called `thepackage` in both Go and Python. `gopls` sets the `name` key as `thepackage.frobinate`, while `pylsp` treats the name as just `frobinate` and puts `thepackage` in `containerName`. This creates an issue where, because we only look at the `name` field when handling `referenced` capabilities, it becomes much harder to search for specific functions. This is something we need to investigate.

### "referencedOutputIgnoreContains" hack

I am currently not happy with `"referencedOutputIgnoreContains"`. This is, admittedly, a hack to remove the returned incidents that are in imported libraries. Take a look at the [InitializeParams struct in the LSP spec](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#initializeParams). Currently, we only set the value `rootUri` as the location for the place we want to analyze. This is a single-valued field, so if we want to make the LSP server aware of more things, such as dependencies or extra workspaces, we have to put it in that folder. A more robust solution, in my opinion, is to take advantage of `workspaceFolders` and create two fields - WorkspaceFolders and DependencyFolders - inside the InitConfig struct. We could concatenate the two lists for LSP requests, and then filter out the elements in DependencyFolders to avoid duplication.