package com.acmeair.web;

import com.acmeair.entities.Customer;
import com.acmeair.entities.CustomerAddress;
import com.acmeair.service.CustomerService;
import javax.servlet.http.HttpServletRequest;
import javax.ws.rs.CookieParam;
import javax.ws.rs.GET;
import javax.ws.rs.POST;
import javax.ws.rs.Path;
import javax.ws.rs.PathParam;
import javax.ws.rs.Produces;
import javax.ws.rs.core.Context;
import javax.ws.rs.core.Response;
import javax.ws.rs.core.Response.Status;
import org.springframework.stereotype.Component;

@Path("/customer")
@Component
public class CustomerREST {
   private CustomerService customerService = (CustomerService)ServiceLocator.getService(CustomerService.class);
   @Context
   private HttpServletRequest request;

   private boolean validate(String customerid) {
      String loginUser = (String)this.request.getAttribute("acmeair.login_user");
      return customerid.equals(loginUser);
   }

   @GET
   @Path("/byid/{custid}")
   @Produces({"application/json"})
   public Response getCustomer(@CookieParam("sessionid") String sessionid, @PathParam("custid") String customerid) {
      try {
         if (!this.validate(customerid)) {
            return Response.status(Status.FORBIDDEN).build();
         } else {
            Customer customer = this.customerService.getCustomerByUsername(customerid);
            return Response.ok(customer).build();
         }
      } catch (Exception var4) {
         var4.printStackTrace();
         return null;
      }
   }

   @POST
   @Path("/byid/{custid}")
   @Produces({"application/json"})
   public Response putCustomer(@CookieParam("sessionid") String sessionid, Customer customer) {
      if (!this.validate(customer.getUsername())) {
         return Response.status(Status.FORBIDDEN).build();
      } else {
         Customer customerFromDB = this.customerService.getCustomerByUsernameAndPassword(customer.getUsername(), customer.getPassword());
         if (customerFromDB == null) {
            return Response.status(Status.FORBIDDEN).build();
         } else {
            CustomerAddress addressFromDB = customerFromDB.getAddress();
            addressFromDB.setStreetAddress1(customer.getAddress().getStreetAddress1());
            if (customer.getAddress().getStreetAddress2() != null) {
               addressFromDB.setStreetAddress2(customer.getAddress().getStreetAddress2());
            }

            addressFromDB.setCity(customer.getAddress().getCity());
            addressFromDB.setStateProvince(customer.getAddress().getStateProvince());
            addressFromDB.setCountry(customer.getAddress().getCountry());
            addressFromDB.setPostalCode(customer.getAddress().getPostalCode());
            customerFromDB.setPhoneNumber(customer.getPhoneNumber());
            customerFromDB.setPhoneNumberType(customer.getPhoneNumberType());
            this.customerService.updateCustomer(customerFromDB);
            customerFromDB.setPassword((String)null);
            return Response.ok(customerFromDB).build();
         }
      }
   }
}
