package com.example.apps;


public class GenericClass<T> {
    private T element;

    public GenericClass(T element) {
        this.element = element;
    }

    public T get() {
        return element;
    }
}
