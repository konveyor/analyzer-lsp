# Rules and pattern reference

This document describes how to write rules and the **pattern syntax** used in `java.referenced` conditions. The pattern system is aligned with the [Eclipse JDT SearchPattern API](https://help.eclipse.org/latest/topic/org.eclipse.jdt.doc.isv/reference/api/org/eclipse/jdt/core/search/SearchPattern.html#createPattern(java.lang.String,int,int,int)) for Java element search.

---

## Overview

Rules use a `when` condition with `java.referenced` to match Java elements (packages, types, methods, fields, annotations, etc.). Each condition specifies:

- **`pattern`** – string pattern that may include wildcards
- **`location`** – which kind of element to match (PACKAGE, IMPORT, TYPE, METHOD, METHOD_CALL, FIELD, ANNOTATION, CLASS)
- **`annotated`** (optional) – for CLASS, METHOD, or FIELD, an extra constraint that the element must be annotated with a given type

Pattern matching is based on Eclipse JDT search: `*` substitutes for zero or more characters; `?` (where supported) substitutes for exactly one character. Match rules such as exact, prefix, and pattern match follow the JDT semantics described below.

---

## Location types and pattern syntax

### PACKAGE

Match package names.

| Pattern style | Example | Description |
|---------------|---------|-------------|
| Exact | `org.springframework.web.servlet` | Matches exactly that package. |
| Wildcard in segment | `org.spri*g*.web.servlet.view.tiles3` | `*` replaces zero or more characters within a segment. |
| Suffix wildcard | `org.springframework.web.servlet*` | Matches the package and subpackages (prefix match). |

```yaml
when:
  java.referenced:
    pattern: org.springframework.web.servlet*
    location: PACKAGE
```

---

### IMPORT

Match import declarations (referenced types).

| Pattern style | Example | Description |
|---------------|---------|-------------|
| Exact type | `org.springframework.web.servlet.ViewResolver` | Matches that type only. |
| Suffix (no dot) | `org.springframework.web.servlet*` | Prefix match on the type name (e.g. types under that package). |

```yaml
when:
  java.referenced:
    pattern: org.apache.log4j.Logger
    location: IMPORT
```

**Note:** IMPORT patterns with an asterisk *after* a dot (e.g. `package.*`) do not work; use patterns like `package*` or exact type names instead.

---

### TYPE

Match type references (class, interface, enum, annotation type).

Pattern syntax follows JDT type patterns:

- **`[qualification '.']typeName ['<' typeArguments '>']`**
- Examples: `java.lang.Object`, `Runnable`, `List<String>`
- Wildcards: `*` in type name for prefix/pattern match

```yaml
when:
  java.referenced:
    pattern: org.springframework.web.multipart.commons.CommonsMultipartResolver
    location: TYPE
```

More complex example: parameterized types, or types with both package and type arguments:

```yaml
when:
  java.referenced:
    pattern: java.util.Map<*, *>
    location: TYPE
```

---

### METHOD

Match method declarations or references.

| Pattern style | Example | Description |
|---------------|---------|-------------|
| Method name only | `do*` | Any method whose name matches the pattern. |
| Simple class + method | `HomeService.do*` | Methods of that class (simple name). |
| FQCN + method | `com.example.service.HomeService.doThings` | Fully qualified declaring type + method name. |
| With type parameters | `com.example.service.HomeService.<T>doThings(T)` | Generic method signature. |

- **Syntax:** `[declaringType '.'] ['<' typeArguments '>'] methodName ['(' parameterTypes ')'] [returnType]`
- Use `*` in method name for prefix or pattern match (e.g. `do*`, `*` for any method).

```yaml
when:
  java.referenced:
    pattern: com.example.service.HomeService.do*
    location: METHOD
```

---

### METHOD_CALL

Match method call sites (invocations).

METHOD_CALL patterns are simpler than METHOD: use declaring type (simple or FQCN) plus method name, with optional `*` wildcards. Generic type parameters, parameter types in parentheses, and return types are **not** supported for METHOD_CALL—only the call site signature (type + method name).

| Pattern style | Example | Description |
|---------------|---------|-------------|
| FQCN + method | `com.example.service.HomeService.doThings` | Exact method call. |
| FQCN + method wildcard | `com.example.service.HomeService.do*` | Any method of that class whose name matches. |
| FQCN + any method | `com.example.service.HomeService.*` | Any method call on that class. |

```yaml
when:
  java.referenced:
    pattern: com.example.service.HomeService.doThings
    location: METHOD_CALL
```

---

### FIELD

Match field references or declarations.

Pattern format: **`[fieldNamePattern] [fieldTypePattern]`**

- **`fieldNamePattern`** – name of the field; `*` matches any name.
- **`fieldTypePattern`** – fully qualified type of the field; wildcards allowed (e.g. `com.example.model.Typed*`).

Examples:

- `'* com.example.model.TypedEntity'` – any field of type `TypedEntity`
- `'* com.example.model.Typed*'` – any field whose type FQCN matches `Typed*`

```yaml
when:
  java.referenced:
    pattern: '* com.example.model.TypedEntity'
    location: FIELD
```

---

### ANNOTATION

Match annotation type references (e.g. on classes or methods).

Use the fully qualified annotation type name. As with TYPE, you can use `*` for prefix or pattern match (e.g. `org.springframework.stereotype.*` to match any annotation in that package).

```yaml
when:
  java.referenced:
    pattern: org.springframework.stereotype.Controller
    location: ANNOTATION
```

---

### Using `annotated` (CLASS, METHOD, FIELD)

The **`annotated`** block adds a constraint that the matched element must be annotated with a given annotation type. It can be used with **CLASS**, **METHOD**, or **FIELD**.

- **`pattern`** – pattern for the class, method, or field itself; `'*'` matches any.
- **`annotated.pattern`** – annotation type the element must carry (wildcards allowed, same as ANNOTATION location).

**CLASS:**

```yaml
when:
  java.referenced:
    pattern: '*'
    location: CLASS
    annotated:
      pattern: org.springframework.stereotype.Controller
```

**METHOD:**

```yaml
when:
  java.referenced:
    pattern: '*'
    location: METHOD
    annotated:
      pattern: org.springframework.context.annotation.Bean
```

You can combine a specific method pattern with an annotation constraint:

```yaml
when:
  java.referenced:
    pattern: '* org.springframework.web.servlet.view.tiles3.TilesConfigurer'
    location: METHOD
    annotated:
      pattern: org.springframework.context.annotation.Bean
```

**FIELD:** use `annotated` with FIELD location to match fields that carry a given annotation (e.g. `@Inject` or custom annotations).


## Pattern syntax by element (JDT summary)

From the JDT API documentation:

- **Type:** `[qualification '.']typeName ['<' typeArguments '>']` — e.g. `java.lang.Object`, `List<String>`. Module form: `[moduleName]/[qualification '.']typeName`.
- **Method:** `[declaringType '.'] ['<' typeArguments '>'] methodName ['(' parameterTypes ')'] [returnType]` — e.g. `java.lang.Runnable.run() void`, `main(*)`.
- **Constructor:** `['<' typeArguments '>'] [declaringQualification '.'] typeName ['(' parameterTypes ')']` — e.g. `java.lang.Object()`, `<Exception>Sample(Exception)`.
- **Field:** `[declaringType '.'] fieldName [fieldType]` — e.g. `java.lang.String.serialVersionUID long`.
- **Package:** `packageNameSegment {'.' packageNameSegment}` — e.g. `java.lang`, `org.e*.jdt.c*e`.

Wildcards: `*` (0 or more characters), `?` (exactly 1 character) where pattern match is used. Type arguments in `<>` do not use `*`; `?` inside `<>` is a wildcard type, not a single-character wildcard.

---

## Example rule file

See `rule.yaml` in this repository for concrete examples of each location and pattern style (PACKAGE, IMPORT, TYPE, METHOD, METHOD_CALL, FIELD, ANNOTATION, and `annotated` with CLASS or METHOD), and `rule.test.yaml` for which patterns are currently covered by tests (including the known limitation for IMPORT with `.*`).

---

## References

- [Eclipse JDT SearchPattern API](https://help.eclipse.org/latest/topic/org.eclipse.jdt.doc.isv/reference/api/org/eclipse/jdt/core/search/SearchPattern.html#createPattern(java.lang.String,int,int,int)) – `createPattern(String, int, int, int)` and match rules.
- `rule.yaml` – pattern examples per location.
- `rule.test.yaml` – test coverage and implementation notes.
