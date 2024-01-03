# Incident Selector

Selecting incidents allows a user to filter what incidents will be added to the violated rules, based on the variables that the provider gives. Any given provider can add any variable they want, so you will need to look up that particular provider's variables to determine what to use. Below we will use the built-in java provider, and its `package` variable to show how to use the incident selector.

## Examples

#### Only include incidents that have a particular package `com.example.apps`

```
--incident-selector='package=com.example.apps'
```

When this is used, **only** the incidents that have the variable of package.example.apps will be included. 

Some packages that would be included:

* com.example.apps.DAO
* com.example.apps

Some packages that will not be included:

* com.example
* com.example.apps2

#### When other providers are used, make sure that their incidents are added

```
--incident-selector='!package || package=com.example.apps'
```

When this is used, any incident that does not have a variable `package` or any of the packages described above will be included.

#### Excluding packages

```
--incident-selector='!package=com.example.apps'
```

In this example, it will be the opposite of what is included section above.

Packages included:

* com.example
* com.example.apps2

Packages excluded:

* com.example.apps.DAO
* com.example.apps
