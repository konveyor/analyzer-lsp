package com.example.apps;

import io.fabric8.kubernetes.api.model.apiextensions.v1beta1.CustomResourceDefinition;

public class App 
{

    /**
     * {@link CustomResourceDefinition}
     * @param args
     */
    public static void main( String[] args )
    {
        CustomResourceDefinition crd = new CustomResourceDefinition();
        System.out.println( crd );

        GenericClass<String> element = new GenericClass<String>("Hello world!");
        element.get();
    }
}
