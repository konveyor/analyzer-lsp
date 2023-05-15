# Labels

Labels are `key=val` pairs associated with Rules or Rulesets. They are specified on a Rule or a Ruleset under `labels` field as a list of strings in `key=val` format:

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
