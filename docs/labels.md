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

## Reserved Labels

The analyzer defines some labels that have special meaning. Here is a list of all such labels:

- `konveyor.io/source`: Identifies source technology a rule or a ruleset applies to.
- `konveyor.io/target`: Identifies target technology a rule or a ruleset applies to.

## Label Selector

The analyzer CLI takes `--label-selector` as an option. It is a string expression that supports logical AND, OR and NOT operations. It can be used to filter-in/filter-out rules based on labels.

To filter-in all rules that have a label with key `konveyor.io/source` and value `eap6`:

```sh
--label-selector="konveyor.io/source=eap6"
```

To filter-in all rules that have a label with key `konveyor.io/source` and any value:

```sh
--label-selector="konveyor.io/source"
```

To perform a logical AND on matches of multiple rules using `&&` operator:

```sh
--label-selector="key1=val1 && key2"
```

To perform a logical OR on matches of multiple rules using `||` operator:

```sh
--label-selector="key1=val1 || key2"
```

To perform a NOT to filter-out rules that have `key1=val1` label set using `!` operator:

```sh
--label-selector="!key1=val1"
```

To group sub-expressions and control precedence using `(` and `)`:

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

