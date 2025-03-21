# Rules

The analyzer rules are a set of instructions that are used to analyze source code and detect issues. Rules are fundamental pieces that codify modernization knowledge.

The analyzer parses user provided rules, evaluates them against input source code and generates _Violations_ for matched rules. A collection of one or more rules form a [Ruleset](#ruleset). _Rulesets_ provide an opionated way of organizing multiple rules that achieve a common goal.

## Table of Contents

1. [Rule Format](#rule)
    1. [Rule Metadata](#rule-metadata)
    2. [Rule Actions](#rule-actions)
        1. [Tag Action](#tag-action)
        2. [Message Action](#message-action)
    3. [Rule Conditions](#rule-conditions)
        1. [Provider Condition](#provider-condition)
        2. [And Condition](#and-condition)
        3. [Or Condition](#or-condition)
2. [Ruleset Format](#ruleset)
3. [Passing rules / rulesets as input](#passing-rules-as-input)

## Rule 

A Rule is written in YAML. It consists of metadata, conditions and actions. It instructs analyzer to take specified actions when given conditions match.

### Rule Metadata

Rule metadata contains general information about a rule:

```yaml
ruleID: "unique_id" (1)
labels: (2)
  - "label1=val1"
effort: 1 (3)
category: mandatory (4)
```

1. **ruleID**: This is a unique ID for the rule. It must be unique within the ruleset.
2. **labels**: A list of string labels associated with the rule. (See [Labels](./labels.md))
3. **effort**: Effort is an integer value that indicates the level of effort needed to fix this issue.
4. **category**: Category describes severity of the issue for migration. Values can be one of _mandatory_, _potential_ or _optional_. (See [Categories](#rule-categories))

#### Rule Categories

* mandatory
  * The issue must be resolved for a successful migration. If the changes are not made, the resulting application will not build or run successfully. Examples include replacement of proprietary APIs that are not supported in the target platform. 
* optional
  * If the issue is not resolved, the application should work, but the results may not be optimal. If the change is not made at the time of migration, it is recommended to put it on the schedule soon after your migration is completed.
* potential
  * The issue should be examined during the migration process, but there is not enough detailed information to determine if the task is mandatory for the migration to succeed.

### Rule description
Each rule should have a `description` field with a short sentence, or "title", summarizing the highlighted problem.

```yaml
description: "Java class foo.Bar has been deprecated"
```

### Rule Actions

A rule has two actions - `tag` and `message`. Either one or two of these actions can be defined on a rule.

#### Tag Action

A tag action is used to create one or more tags for an application when the rule matches. It takes a list of string tags as its fields:

```yaml
tag:
  # tags can be comma separated
  - "tag1,tag2,tag3"
  # optionally, tags can be assigned categories
  - "Category=tag4,tag5"
```

When a tag is a key=val pair, the keys are treated as category of that tag. For instance, `Backend=Java` is a valid tag with `Backend` being the category of tag `Java`.

> Any rule that has a tag action in it is referred to as a "tagging rule".

#### Message Action

A message action is used to create an issue with the specified message when a rule matches:

```yaml
# when a match is found, analyzer generates incidents each having this message
message: "helpful message about the violation"
```

Message can also be templated to include information about the match interpolated via custom variables on the rule (See [Custom Variables](#custom-variables)):

```
- ruleID: lang-ref-004
   customVariables:
   - pattern: '([A-z]+)\.get\(\)'
      name: VariableName
    message: "Found generic call - {{ VariableName }}"
  when:
    <CONDITION>
```

##### Links

Hyperlinks can be provided along with a `message` or `tag` action to provide relevant information about the found issue: 

```yaml
# links point to external hyperlinks
# rule authors are expected to provide
# relevant hyperlinks for quick fixes, docs etc
links:
  - url: "konveyor.io"
    title: "short title for the link"
```

### Rule Conditions

Every rule has a `when` block that contains exactly one condition. A condition defines a search query to be evaluated against the input source code. 

```yaml
when:
  <condition>
```

There are three types of conditions - _and_, _or_ and _provider_. While the _provider_ condition is responsible for performing an actual search in the source code, the _and_ and _or_ conditions are logical constructs provided by the engine to form a complex condition from the results of multiple other conditions.

#### Provider Condition

The analyzer engine enables multi-language source code analysis via something called as “providers”. A "provider" knows how to analyze the source code of a technology. It publishes what it can do with the source code in terms of "capabilities".

A provider condition instructs the analyzer to invoke a specific "provider" and use one of its "capabilities". In general, it is of the form `<provider_name>.<capability>`:

For instance, the `java` provider provides `referenced` capability. To search through Java source code, we can write a `java.referenced` condition:

```yaml
when:
  java.referenced:
    pattern: org.kubernetes*
    location: IMPORT
```

Note that depending on the provider, the fields of the condition (for instance, pattern and location above) will change.

Some providers have _dependency_ capability. It means that the provider can generate a list of dependencies for a given application. A dependency condition can be used to query this list and check whether a certain dependency (with a version range) exists for the application. For instance, to check if a Java application has a certain dependency, we can write a `java.dependency` condition:

```yaml
when:
  java.dependency:
    name: junit.junit
    upperbound: 4.12.2
    lowerbound: 4.4.0
```

Analyzer currently supports `builtin`, `java`, `go` and `generic` providers. Here is the table that summarizes all the providers and their capabilities:

| Provider Name | Capabilities                                                  | Description                                                                       |
| ------------- | ------------------------------------------------------------- | --------------------------------------------------------------------------------- |
| java          | referenced                                                    | Find references of a pattern with an optional code location for detailed searches |
|               | dependency                                                    | Check whether app has a given dependency                                          |
| builtin       | xml                                                           | Search XML files using xpath queries                                              |
|               | json                                                          | Search JSON files using jsonpath queries                                          |
|               | filecontent                                                   | Search content in regular files using regex patterns                              |
|               | file                                                          | Find files with names matching a given pattern                                    |
|               | hasTags                                                       | Check whether a tag is created for the app via a tagging rule                     |
| go            | referenced                                                    | Find references of a pattern                                                      |
|               | dependency                                                    | Check whether app has a given dependency                                          |

Based on the table above, we should be able to create the first part of the condition that doesn’t contain any of the condition fields. For instance, to create a `java` provider condition that uses `referenced` capability:

```yaml
when:
  java.referenced:
    <fields>
```

Depending on the _provider_ and the _capability_, there will be different `<fields>` in the condition. Following table summarizes available providers, their capabilities and all of their fields:

| Provider | Capability  | Fields      | Required | Description                                                                                   |
|----------|-------------|-------------|----------|-----------------------------------------------------------------------------------------------|
| java     | referenced  | pattern     | Yes      | Regex pattern                                                                                 |
|          |             | location    | No       | Source code location (see [Java Locations](#java-locations))                                  |
|          |             | annotated   | No       | Additional query to inspect annotations (see [Annotation inspection](#annotation-inspection)) |
|          | dependency  | name        | Yes      | Name of the dependency                                                                        |
|          |             | nameregex   | No       | Regex pattern to match the name                                                               |
|          |             | upperbound  | No       | Match versions lower than or equal to                                                         |
|          |             | lowerbound  | No       | Match versions greater than or equal to                                                       |
| builtin  | xml         | xpath       | Yes      | Xpath query                                                                                   |
|          |             | namespaces  | No       | A map to scope down query to namespaces                                                       |
|          |             | filepaths   | No       | Optional list of files to scope down search                                                   |
|          | json        | xpath       | Yes      | Xpath query                                                                                   |
|          |             | filepaths   | No       | Optional list of files to scope down search                                                   |
|          | filecontent | pattern     | Yes      | Regex pattern to match in content                                                             |
|          |             | filePattern | No       | Only search in files with names matching this pattern                                         |
|          | file        | pattern     | Yes      | Find files with names matching this pattern                                                   |
|          | hasTags     |             |          | This is an inline list of string tags. See [Tag Action](#tag-action)                          |
| go       | referenced  | pattern     | Yes      | Regex pattern                                                                                 |
|          | dependency  | name        | Yes      | Name of the dependency                                                                        |
|          |             | nameregex   | No       | Regex pattern to match the name                                                               |
|          |             | upperbound  | No       | Match versions lower than or equal to                                                         |
|          |             | lowerbound  | No       | Match versions greater than or equal to                                                       |


With the information above, we should be able to complete `java` condition we created earlier. We will search for references of a package:

```yaml
when:
  java.referenced:
    location: PACKAGE
    pattern: org.jboss*
```

##### Java Locations

The java provider allows scoping the search down to certain source code locations. Any one of the following search locations can be used to scope down java searches:

- **IMPORT**: IMPORT allows for searches on class imports. It can either be used with FQNs or an asterisk to allow for wider matches:
```yaml
java.referenced:
  pattern: org.apache.lucene.search*
  location: IMPORT
```
would match on each of these imports:
```java
import org.apache.lucene.search.Query;
import org.apache.lucene.search.Sort;
import org.apache.lucene.search.SortField;
```
:warning: If you want to match using an asterisk (`*`) for a wider range of results, it is recommended to place it directly after the package, not after the dot:

:no_entry_sign: `org.apache.lucene.search.*`
:white_check_mark: `org.apache.lucene.search*`

- **PACKAGE**: the PACKAGE location matches on any usage of a package, be it in an import or used as part of a fully qualified name in the code:
```yaml
java.referenced:
  pattern: org.apache.lucene.search*
  location: PACKAGE
```
would match on both the import and the fully qualified usage:
```java
import org.apache.lucene.search.*;
```
```java
public class Test {
  private org.apache.lucene.search.Query query;
}
```
:warning: As in the IMPORT condition, try to avoid using asterisk (`*`) right after the package-separation dot (`.`) for better results.

- **CONSTRUCTOR_CALL** and **METHOD_CALL**: for matching constructors and methods respectively. The pattern possibilities are quite varied,
and it is possible to match against specific return types, arguments, etc.

For instance, looking for a method named "method" declared on `org.konveyor.MyClass` that returns
a `List` of a type that extends `java.lang.String` and accepts a single parameter:
```yaml
java.referenced:
  location: METHOD
  pattern: 'org.konveyor.Myclass.method(*) java.util.List<? extends java.lang.String>'
```
More information about the possibilities of these patterns can be found in
[the official javadocs contain all the information for building these patterns](https://help.eclipse.org/latest/index.jsp?topic=%2Forg.eclipse.jdt.doc.isv%2Freference%2Fapi%2Forg%2Feclipse%2Fjdt%2Fcore%2Fsearch%2FSearchPattern.html&anchor=createPattern(java.lang.String,int,int,int))
in the `createPattern(String, int, int, int)` section.

:warning: At the moment, fully qualified static method matching is prone to errors.

- **TYPE**: matches against types in general, appearing anywhere.
- **INHERITANCE**: matches against a class inheriting from a given type.
- **ANNOTATION**: matches against annotations.
- **IMPLEMENTS_TYPE**: matches against any type implementing the given type.
- **ENUM_CONSTANT**: matches against enum constants.
- **RETURN_TYPE**: matches against a type being returned by a method.
- **VARIABLE_DECLARATION**: matches against a type being declared as a variable.
- **FIELD** (declaration): matches against a type appearing in a field declaration. Can be coupled with an annotation match, this is, an annotation happening on the field (see [Annotation inspection](#annotation-inspection))
- **METHOD**: matches against a given method declaration. Can be coupled with an annotation match (see [Annotation inspection](#annotation-inspection)).
- **CLASS** (declaration): matches against a given method declaration. Can be coupled with an annotation match (see [Annotation inspection](#annotation-inspection)).


##### Annotation inspection
It is possible to add a query to match against specific annotations and their elements. For instance:

```yaml
when:
  java.referenced:
    location: METHOD
    pattern: org.package.MyApplication.runApplication(java.lang.String)
    annotated:
      pattern: org.framework.Bean
      elements:
      - name: url
        value: "http://www.example.com"
```
would match against the `runApplication` method in the following java code:
```java
package org.package

import org.framework.Bean;

class MyApplication {

    @Bean(url = "http://www.example.com")
    public String runApplication(String str) {
        // ...
    }
}
```
The structure of the `annotated` YAML element is the following:
```yaml
annotated:
  pattern: a Java regex to match the fully qualified name of the annotation (optional)
  elements: an array of elements to match within the annotation (optional)
  - name: the exact name of the element to match against
    value: a Java regex to match the value of the element
```

It is also possible to match an annotation with specific elements, without having to specify the symbol it annotates.
The following example would also match on the `@Bean` annotation in the same code as the last example. Note that
the only element specified with a `pattern` is the annotation itself:
```yaml
when:
  java.referenced:
    location: ANNOTATION
    pattern: org.framework.Bean
    annotated:
      elements:
        - name: url
          value: "http://www.example.com"
```

##### Condition patterns
The Language Server used by the Java provider is Eclipse's JDTLS. Internally, the JDTLS uses the Eclipse Java Development Toolkit,
which includes utilities for searching code in projects. In the `pattern` element of a `java.referenced` condition, we can therefore
search code using these utilities.
[The official javadocs contain all the information for building these patterns](https://help.eclipse.org/latest/index.jsp?topic=%2Forg.eclipse.jdt.doc.isv%2Freference%2Fapi%2Forg%2Feclipse%2Fjdt%2Fcore%2Fsearch%2FSearchPattern.html&anchor=createPattern(java.lang.String,int,int,int))
in the `createPattern(String, int, int, int)` section.

Here are some examples of what can be used:

- Look for any class under the `javax.xml` package, occurring in any location:
```yaml
java.referenced:
  pattern: javax.xml*
```
:warning: when matching against packages, as in the previous example, the asterisk must not be after a dot:
- :green_circle: `pattern: javax.xml*`
- :red_circle: `pattern: javax.xml.*`

- Look for method declarations that return `java.lang.String`:
```yaml
java.referenced:
  location: METHOD
  pattern: '* java.lang.String'
```

- Look for a method named "method" declared on `org.konveyor.MyClass` that returns a `List` of a type that extends `java.lang.String`:
```yaml
java.referenced:
  location: METHOD
  pattern: 'org.konveyor.Myclass.method(*) java.util.List<? extends java.lang.String>'
```

- Look for a class that implements `java.util.List`:
```yaml
java.referenced:
  location: IMPLEMENTS_TYPE
  pattern: java.util.List
```



##### Custom Variables

Provider conditions can have associated "custom variables". Custom variables are used to capture relevant information from the matched line in the source code. The values of these variables will be interpolated with data matched in the source code. These values can be used to generate detailed templated messages in a rule’s action (See [Message action](#message-action)). They can be added to a rule in the `customVariables` field:

```yaml
- ruleID: lang-ref-004
   customVariables:
   - pattern: '([A-z]+)\.get\(\)' (1)
      name: VariableName (2)
    message: "Found generic call - {{ VariableName }}" (3)
  when:
      java.referenced:
          location: METHOD_CALL
          pattern: com.example.apps.GenericClass.get
```

1. **pattern**:  This is a regex pattern that will be matched on the source code line when a match is found.
2. **name**:  This is the name of the variable that can be used in templates.
3. **message**: This is how to template a message using a custom variable.

#### And Condition

The `And` condition takes an array of conditions and performs a logical 
"and" operation on their results:

```yaml
when:
  and:
    - <condition1>
    - <condition2>
```

Example:

```yaml
when:
  and:
    - java.dependency:
        name: junit.junit
        upperbound: 4.12.2
        lowerbound: 4.4.0
    - java.dependency:
        name: io.fabric8.kubernetes-client
        lowerbound: 5.0.100
```

Note that the conditions can also be nested within other conditions:

```yaml
when:
  and:
  - and:
    - go.referenced: "*CustomResourceDefinition*"
    - java.referenced:
        pattern: "*CustomResourceDefinition*"
  - go.referenced: "*CustomResourceDefinition*"
```

#### Or Condition

The `Or` condition takes an array of other conditions and performs a logical "or" operation on their results:

```yaml
when:
  or:
    - <condition1>
    - <condition2>
```

#### Chaining Condition Variables

It is also possible to use the output of one condition as the input for filtering another one in an and/or condition. This is called
*condition chaining*.

The OpenAPI spec contains more information about what each condition can offer in its output; check the `<provider>.<condition>.out` component.

Example:

```yaml 
when:
 or:
  - builtin.xml:
      xpath: "//dependencies/dependency"
      filepaths: "{{poms.filepaths}}"
    from: poms
  - builtin.file:
      pattern: pom.xml
    as: poms
    ignore: true
```

In the above example the output of `builtin.file` condition is saved `as` poms. 

```yaml
      as: poms
```

The variables of `builtin.file` can then be used in the `builtin.xml` condition, by writing `from` and then using [mustache templates](https://mustache.github.io/mustache.5.html) in the _provider_ condition` block.

This is how this particular condition knows how to use the variables set to the name `poms`. 

```yaml
    from: poms
```

Then you can use the variables by setting them as mustached templates in any of the inputs to the _provider_ condition.

```yaml
      filepaths: "{{poms.filepaths}}"
```

**Note**: If you only want to use the values of a condition as a chain, you can set `ignore: true`, this will tell the engine to not use this condition to determine if the rule has violated or not. example above:
```yaml
    ignore: true
``` 

#### Note about chaining in the Java provider
In the java provider, the `filepaths` variable must be uppercased. For instance:
```yaml
  when:
    and:
      - java.referenced:
          pattern: org.springframework.web.bind.annotation.RequestMapping
          location: ANNOTATION
        as: annotation
      - java.referenced:
          pattern: org.springframework.stereotype.Controller
          location: ANNOTATION
          filepaths: "{{annotation.Filepaths}}"
```


## Ruleset

A set of Rules form a Ruleset. Rulesets are an opinionated way of passing Rules to Rules Engine.

A ruleset is created by placing one or more YAML rules files in a directory and creating a `ruleset.yaml` (golden file) file in it. 

The golden file stores metadata of the Ruleset.

```yaml
name: my-ruleset (1)
description: Text description about ruleset (2)
labels: (3)
- key=val
```

1. **name**: This is a requried field. A unique name for the ruleset.
2. **description**: This is a requried field. Text description about the ruleset.
3. **labels**: This is an optional field. A list of string labels for the ruleset. The labels on a ruleset are automatically inherted by all rules in the ruleset. (See Labels)

## Passing rules as input

The analyzer CLI provides `--rules` option to specify a YAML file containing rules or a ruleset directory:

- It can be a file:
  ```sh
  konveyor-analyzer --rules rules-file.yaml ...
  ```
  It is assumed that the file contains a list of YAML rules. The engine will automatically associate all rules in it with a default _Ruleset_.

- It can be a directory:
  ```sh
  konveyor-analyzer --rules /ruleset/directory/ ...
  ```
  It is assumed that the directory contains a _Ruleset_. (See [Ruleset](#ruleset))

- It can be given more than once with a mix of rules files and rulesets:
  ```sh
  konveyor-analyzer --rules /ruleset/directory/ --rules rules-file.yaml ...
  ```