package com.acmeair.web.config;

import org.springframework.context.annotation.ComponentScan;
import org.springframework.context.annotation.Configuration;
import org.springframework.context.annotation.ImportResource;

@Configuration
@ImportResource({"classpath:/spring-config-acmeair-data-jpa.xml"})
@ComponentScan(
   basePackages = {"com.acmeair.jpa.service"}
)
public class WXSDirectAppConfig {
}
