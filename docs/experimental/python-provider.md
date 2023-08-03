# Python Provider using Generic Provider

We are using the jedi-language-server (https://github.com/pappasam/jedi-language-server) to make a python provider using the generic provider.

jedi-language-server can be installed using

```
pip install jedi-langauge-server
```

It will be installed in `/home/<user_name>/.local/bin/jedi-language-server`

It will run without any arguments, but for more information it can be run with `--log-file LOG_FILE --verbose`

The configuration used was:

```json
    {
        "name": "python",
        "binaryPath": "/path/to/generic/provider/binary",
        "initConfig": [{
            "location": "examples/python",
            "analysisMode": "full",
            "providerSpecificConfig": {
                "name": "python",
                "lspServerPath": "/path/to/jedi/language/server",
            }
        }]
    },
```

The rule used to test it out was:

```yaml
    - message: python sample rule
    ruleID: python-sample-rule-001
    when:
        python.referenced: 
            pattern: "create_custom_resource_definition"
```

The example used for testing was:
```python
    #!/usr/bin/env python

    import kubernetes

    def main():
        print(kubernetes.client.ApiextensionsV1beta1Api.create_custom_resource_definition)

    if __name__ == '__main__':
        main()
```

## Findings

The jedi-language-server was able to get initialized and communicate with the analyzer-lsp.

However, it returned `null` as a response to the rule. 

After further testing, it was found that the jedi-language-server isn't able to find references to imported functions. 

jedi-language-server returned a response when the rule was

```yaml
    - message: python sample rule
    ruleID: python-sample-rule-001
    when:
        python.referenced: 
            pattern: "main"
```

## Results

We are going to move onto a different language server to test out the Generic Provider. 

We are also going to investigate the behaviour of jedi-language-server to see whether not recognizing imported functions is intended. And also investigate GoPls to see whether recognizing imported functions is intended. 