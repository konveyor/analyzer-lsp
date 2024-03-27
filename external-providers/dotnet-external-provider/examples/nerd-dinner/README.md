# Nerd Dinner

This example demonstrates our ability to perform rules based analys of a .NET
Framework 4.5 project like [nerd-dinner](https://github.com/sixeyed/nerd-dinner)
given a Windows host capable of building it.

# Procedure

### Prepare the Host

Since .NET Framework 4.5 is a Windows only framework, our provider MUST run on
a Windows host prepared to build the project we want to analyze (in our case
nerd-dinner).

#### Install Language Server

Install the [csharp-language-server](https://github.com/razzmatazz/csharp-language-server).

#### Download the .NET Provider

...Instructions for getting the dotnet-external-provider binary for windows...

#### Clone the Repository

...

#### Install .NET SDK(s)

...

#### Make Host Accessible

We must make it so that, when we start the provider to listen for traffic, that
outside traffic will make it to our provider.

### Analyze the Project

At this point we have a Windows host with our nerd-dinner project cloned and
all of the necessary software to build and analyze it installed. Now all we
need to do is:

1. Start the provider.
1. Write the provider settings file.
1. Run the analysis

# Conclusion

This is awesome right?
