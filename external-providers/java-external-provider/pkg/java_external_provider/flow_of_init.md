# Init Flow Write Up

## Getting all the Config

The first thing that we do is get all of the config, and the provider specifics
into variables and set the defaults if not present.

## If location is mvn://

If the location is a something to download from maven then we will start there by
downloading from maven. This process takes about 60 lines of code all in. This probably
should be encapsulated by something.

## Load the opensource dep labels (Func)

Here we pull the maven index file and load the groups into a hashmap for 
easy look up.

## If .jar, .war, .ear then decompile

This will decompile and update the config based on that docmpilation. 

**Note: that this uses a switch with a single case**

### Decompile Java (Func)

Creates the new java-project that we will use.

#### Expload the archive (Func)

Expload will open the archive zip and loop through the files, copying them out to the a new "exploaded" dir.

##### Determine what to do with the file

###### Class file in WEB-INF or META-INF

here we create a decompile job to decompile the class file and put it in the correct source code directory.

###### Class file not in WEB-INF or META-INF

here we create a decompile job to put the code in the java project, but we also create a "dependency" for it.

**NOTE: this is a bug, because jar Files don't have user code in META-INF or WEB-INF**

###### If it is a java file

Then we copy the java file from the exploaded path to the correct source code location

###### If it is a War File

Then we will recursivally call expload

###### If it is a Jar File

We try and create a dependency for that jar

####### Maven Search

If allowed will try and use maven.search.org

####### From Pom

If maven search is not allowed or if it fails, we will try and find the 
pom.properties and use that

####### From Directory Structure

If these don't work, then we will try to get a group id from the maven index and set the artificat ID to the jar name

After trying to get the dependnecy info, if we fail with error, we will decompile it and add to the source code repo

If we did find the dependency in maven.search we will copy the jar to .m2 cache

If we didn't we will create a decompile job that will create a *-sources.jar

if we fail with no error and no dep then we will copy the jar file .m2 repo

###### Default case

We move the file from the exploaded path to the project to the same directory.

#### Create Java Prject

This will take the found dependencies and create a new pom.xml file from them.

## Set up Proxy env vars

There is a bug here, because the proxy is set after the attempt to download the jar from
maven and this means it won't use the configured proxy.

## Set Up and Start JDTLS

Next we do all the things that are needed to get the jdtls started

## Next we create the javaServiceClient Struct

Just putting all the values into the struct

## Next we determine if we need to resolve sources

Resolving sources for either Maven or Gradle

### Resolving Maven Sources

We will call the specific maven command to resolve deps and download sources

If any of the deps sources are not found, then we will decompile them

#### Decompile

takes a list of jobs, creates workers and then starts to decompile.

##### Expload if Java Archive was decompiled

See Exploaded Funcion Above

### Resolving Gradle Sources

We use a builtin gradle task installed at /root/.gradle/task.gradle

This is responsible for resolving deps and then downloading sources.

If sources are not found then we will decompile them See Decompile above.

## Initialize the svc client

## Set Dep labels on the svc client

## Init Exluded Dep Labels

and set that on the svcClient.

## Set up JDTLS log forwarder

we have a 20 line piece of code, that starts a log follow on .metadata.log
and then will add a log if it contains the text "KONVEYOR_LOG"


## Issues with flow

1. It is disjointed, hard to follow and has lots of caveats and side effects.
2. Is slow because we are decompile individual files, rather then whole things.
3. It is not clear what goes where and when and why, this leads to lots of overhead when debugging because you have to keep track of it all
and the code that determines it is a bunch of string manipulation that isn't clear IMO.
4. There are genuine differences in War, Ear, Jar that are not captured by the flow.
5. It is not clear, that resolving dependencies is used by decompiling the binary, to pull dependencies
6. resolving dependencies uses the same exlpoadsion code, when I am pretty sure that we don't need to do this.
Once fern-flower decompiles the full jar, it is a sources jar. 
7. We end up putting things into the source tree that shouldn't be.
8. the decompile jobs are run twice, but have worker fan-out/fan-in arch. This is kind of confusing.


### Thoughts on how to make it better

