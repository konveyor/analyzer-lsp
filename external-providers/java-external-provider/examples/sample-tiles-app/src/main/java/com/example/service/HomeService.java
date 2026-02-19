package com.example.service;

import com.example.model.B;
import com.example.model.TypedEntity;
import org.springframework.core.AttributeAccessor;

public class HomeService implements AttributeAccessor {

    private TypedEntity<B> typedEntity;

    public HomeService() {
        this.typedEntity = new TypedEntity<>(new B());
    }

    public void doStuff() {
        B b = new B();
        TypedEntity<B> typedEntity = new TypedEntity<>(b);
    }

    public <T extends B> String doThings(T input) {
        return "Hello World!";
    }

    @Override
    public void setAttribute(String name, Object value) {

    }

    @Override
    public Object getAttribute(String name) {
        return null;
    }

    @Override
    public Object removeAttribute(String name) {
        return null;
    }

    @Override
    public boolean hasAttribute(String name) {
        return false;
    }

    @Override
    public String[] attributeNames() {
        return new String[0];
    }
}
