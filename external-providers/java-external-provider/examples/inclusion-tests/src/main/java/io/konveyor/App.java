package io.konveyor;

import java.io.File;
import io.konveyor.util.FileReader;

public class App 
{

    /**
     * {@link CustomResourceDefinition}
     * @param args
     */
    public static void main( String[] args )
    {
        if (FileReader.fileExists()) {
            File file = new File("/test");
        }
    }
}
