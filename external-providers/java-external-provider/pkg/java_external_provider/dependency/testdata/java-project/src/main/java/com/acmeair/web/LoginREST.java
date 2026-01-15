package com.acmeair.web;

import com.acmeair.entities.CustomerSession;
import com.acmeair.service.CustomerService;
import javax.ws.rs.Consumes;
import javax.ws.rs.CookieParam;
import javax.ws.rs.FormParam;
import javax.ws.rs.GET;
import javax.ws.rs.POST;
import javax.ws.rs.Path;
import javax.ws.rs.Produces;
import javax.ws.rs.QueryParam;
import javax.ws.rs.core.NewCookie;
import javax.ws.rs.core.Response;
import javax.ws.rs.core.Response.Status;
import org.springframework.stereotype.Component;

@Path("/login")
@Component
public class LoginREST {
   public static String SESSIONID_COOKIE_NAME = "sessionid";
   private CustomerService customerService = (CustomerService)ServiceLocator.getService(CustomerService.class);

   @POST
   @Consumes({"application/x-www-form-urlencoded"})
   @Produces({"text/plain"})
   public Response login(@FormParam("login") String login, @FormParam("password") String password) {
      try {
         boolean validCustomer = this.customerService.validateCustomer(login, password);
         if (!validCustomer) {
            return Response.status(Status.FORBIDDEN).build();
         } else {
            CustomerSession session = this.customerService.createSession(login);
            NewCookie sessCookie = new NewCookie(SESSIONID_COOKIE_NAME, session.getId());
            return Response.ok("logged in").cookie(new NewCookie[]{sessCookie}).build();
         }
      } catch (Exception var6) {
         var6.printStackTrace();
         return null;
      }
   }

   @GET
   @Path("/logout")
   @Produces({"text/plain"})
   public Response logout(@QueryParam("login") String login, @CookieParam("sessionid") String sessionid) {
      try {
         this.customerService.invalidateSession(sessionid);
         NewCookie sessCookie = new NewCookie(SESSIONID_COOKIE_NAME, "");
         return Response.ok("logged out").cookie(new NewCookie[]{sessCookie}).build();
      } catch (Exception var4) {
         var4.printStackTrace();
         return null;
      }
   }
}
