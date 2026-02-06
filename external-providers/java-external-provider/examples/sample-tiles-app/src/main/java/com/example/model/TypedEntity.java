package com.example.model;

public class TypedEntity<T extends A> {
    T value;

    public TypedEntity(T value) {
        this.value = value;
    }

    public T getValue() {
        return value;
    }

    public void setValue(T value) {
        this.value = value;
    }
}
