# Labels

Labels are `key=val` pairs associated with Rules or Rulesets as well as Dependencies. They are specified on a Rule or a Ruleset. For Dependencies a provider will add the labels to the dependecies when retrieving them. Labels on a Ruleset are automatically inherited by all the Rules in it. Labels are specified under `labels` field as a list of strings in `key=val` format:

```yaml
labels:
- "key1=val1"
- "key2=val2"
```

The key of a label can be subdomain-prefixed:

```yaml
labels:
- "konveyor.io/key1=val1"
```

The value of a label can be empty:

```yaml
labels:
- "konveyor.io/key="
```

The value of a label can be omitted, it will be treated as an empty value:

```yaml
labels:
- "konveyor.io/key"
```

## Rule Labels

The analyzer defines some labels that have special meanings:

- `konveyor.io/source`: Identifies source technology a rule or a ruleset applies to. The value can be a string with optional version range at the end e.g. "eap", "eap6", "eap7-" etc.
- `konveyor.io/target`: Identifies target technology a rule or a ruleset applies to. The value can be a string with optional version range at the end e.g. "eap", "eap6", "eap8+" etc.
- `konveyor.io/include`: Overrides filter behavior for a rule irrespective of the label selector used. The value can either be `always` or `never`. `always` will always filter-in this rule, `never` will always filter-out this rule.  

### Rule Label Selector

The analyzer CLI takes `--label-selector` as an option. It is a string expression that supports logical AND, OR and NOT operations. It can be used to filter-in/filter-out rules based on labels.

Here are different scenarios of how rules will be filtered in or out based on a label selector expression:

* _Exact label value match_

  * To filter-in all rules that have a label with key `konveyor.io/source` and value `kubernetes`:
  
    ```sh
    --label-selector="konveyor.io/source=kubernetes"
    ```
    The value `kubernetes` must be an exact string match in this case.
    
    With the above label selector, only the rule `rule-000` will be matched from the following input rules: 

    ```yaml
    - ruleID: rule-000
      labels:
      - konveyor.io/source=kubernetes
      [...]
    - ruleID: rule-001
      labels:
      - konveyor.io/source=openshift
      [...]
    ```

* _Any label value match_
  
  * To filter-in all rules that have a label with key `konveyor.io/source` and _any_ value:

    ```sh
    --label-selector="konveyor.io/source"
    ```
  
    Only the key `konveyor.io/source` should be present in the rule labels no matter what value.

    Both the rules `rule-000` and `rule-001` in the following input rules with the above label selector:

    ```yaml
    - ruleID: rule-000
      labels:
      - konveyor.io/source=kubernetes
      [...]
    - ruleID: rule-001
      labels:
      - konveyor.io/source=openshift
      [...]
    ```

  * Some rules themselves have labels with only keys and no values. For instance:

    ```yaml
    - ruleID: rule-000
      labels:
      - konveyor.io/source
      [...]
    ```
    Such rules will match on any value of the `konveyor.io/source` label provided in the label selector expression.

    For instance, the rule `rule-000` above will match when the input expression is as follows:
    
    ```sh
    --label-selector konveyor.io/source=kubernetes
    ```

* _Logical AND between multiple labels_

  * To perform a logical AND between results of multiple label matches using `&&` operator:
    
    ```sh
    --label-selector="konveyor.io/target=kubernetes && component=storage"
    ```
    
    Only the rule `rule-001` from the following rules will match with the above label selector:

    ```yaml
    - ruleID: rule-001
      labels:
      - konveyor.io/target=kubernetes
      - component=storage
      [...]
    - ruleID: rule-002
      labels:
      - konveyor.io/target=kubernetes
      [...]
    ```

* _Logical OR between multiple labels_
  
  * To perform a logical OR between results of multiple label matches using `||` operator:

    ```sh
    --label-selector="konveyor.io/target=kubernetes || konveyor.io/target=openshift"
    ```
  
    Both the rules `rule-001` and `rule-002` will be matched with the above label selector:
  
    ```yaml
        - ruleID: rule-001
        labels:
        - konveyor.io/source=kubernetes
        [...]
      - ruleID: rule-002
        labels:
        - konveyor.io/source=openshift
        [...]
    ```
  

* _Logical NOT to filter-out a rule_

  * Label selector can also be used to exclude rules using the `!` operator:
    
    ```sh
    --label-selector="!component=network && konveyor.io/target=kubernetes"
    ```

    From the following rules, the rules `rule-001` and `rule-003` will be matched with the above label selector:

    ```yaml
        - ruleID: rule-001
        labels:
        - konveyor.io/target=kubernetes
        - component=storage
        [...]
      - ruleID: rule-002
        labels:
        - konveyor.io/target=kubernetes
        - component=network
        [...]
      - ruleID: rule-003
        labels:
        - konveyor.io/target=kubernetes
        - component=compute
        [...]
    ```    


* _Grouping subexpressions_

  * To group sub-expressions and control precedence using `(` and `)`:

    ```sh
    --label-selector="(key1=val1 || key2=val2) && !val3"
    ```

## Dependency Labels

The analyzer engine adds labels on dependencies. These labels provide additional information about a dependency such as whether it's open-source or internal, programming language, etc. 

Currenty, analyzer adds following labels on dependencies:

```yaml
labels:
- konveyor.io/dep-source=internal
- konveyor.io/language=java
```

### Dependency Label Selector

Analyzer CLI accepts `--dep-label-selector` option that allows filtering-in / filtering-out incidents generated from a dependency based on the labels.

For instance, analyzer adds `konveyor.io/dep-source` label on dependencies with a value that identifies whether the dependency is a known open source dependency or not. To exclude incidents for all such open-source dependencies, `--dep-label-selector` can be used as:

```sh
konveyor-analyzer ... --dep-label-selector !konveyor.io/dep-source=open-source
```

The Java provider in analyzer also takes a list of packages to add an exclude label. To exclude all such packages, `--dep-label-selector` can be used as:

```sh
konveyor-analyzer ... --dep-label-selector !konveyor.io/exclude
```

