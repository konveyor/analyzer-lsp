package com.acmeair.web;

import com.acmeair.web.config.WXSDirectAppConfig;
import java.util.concurrent.atomic.AtomicReference;
import java.util.logging.Logger;
import javax.naming.Context;
import javax.naming.InitialContext;
import javax.naming.NamingException;
import org.springframework.context.ApplicationContext;
import org.springframework.context.annotation.AnnotationConfigApplicationContext;

public class ServiceLocator {
   public static String REPOSITORY_LOOKUP_KEY = "com.acmeair.repository.type";
   final ApplicationContext ctx;
   private static Logger logger = Logger.getLogger(ServiceLocator.class.getName());
   private static AtomicReference singletonServiceLocator = new AtomicReference();

   static ServiceLocator instance() {
      if (singletonServiceLocator.get() == null) {
         synchronized(singletonServiceLocator) {
            if (singletonServiceLocator.get() == null) {
               singletonServiceLocator.set(new ServiceLocator());
            }
         }
      }

      return (ServiceLocator)singletonServiceLocator.get();
   }

   private ServiceLocator() {
      String type = null;
      String lookup = REPOSITORY_LOOKUP_KEY.replace('.', '/');
      Context context = null;

      try {
         context = new InitialContext();
         Context envContext = (Context)context.lookup("java:comp/env");
         if (envContext != null) {
            type = (String)envContext.lookup(lookup);
         }
      } catch (NamingException var7) {
      }

      if (type != null) {
         logger.info("Found repository in web.xml:" + type);
      } else if (context != null) {
         try {
            type = (String)context.lookup(lookup);
            if (type != null) {
               logger.info("Found repository in server.xml:" + type);
            }
         } catch (NamingException var6) {
         }
      }

      if (type == null) {
         type = System.getProperty(REPOSITORY_LOOKUP_KEY);
         if (type != null) {
            logger.info("Found repository in jvm property:" + type);
         } else {
            type = System.getenv(REPOSITORY_LOOKUP_KEY);
            if (type != null) {
               logger.info("Found repository in environment property:" + type);
            }
         }
      }

      type = "wxsdirect";
      logger.info("Using default repository :" + type);
      this.ctx = new AnnotationConfigApplicationContext(new Class[]{WXSDirectAppConfig.class});
   }

   public static Object getService(Class clazz) {
      return instance().ctx.getBean(clazz);
   }
}
