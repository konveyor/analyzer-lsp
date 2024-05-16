package com.example.apps.util;

import java.io.File;

public class FileReader {
    public static void main(String[] args) {
        File file = new File("test");
        if (file.exists()) {
            System.out.println("file exists");
        }
    }

}
