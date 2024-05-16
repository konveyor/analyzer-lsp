package com.example.apps;

import java.io.File;
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

        // test file usage
        File file = new File("test");
        if (file.exists()) {
            System.out.println("file exists");
        }
    }
}
