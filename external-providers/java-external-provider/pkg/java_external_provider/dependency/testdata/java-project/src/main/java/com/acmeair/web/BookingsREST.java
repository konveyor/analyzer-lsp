package com.acmeair.web;

import com.acmeair.entities.Booking;
import com.acmeair.entities.BookingPK;
import com.acmeair.entities.FlightPK;
import com.acmeair.service.BookingService;
import java.util.List;
import javax.ws.rs.Consumes;
import javax.ws.rs.FormParam;
import javax.ws.rs.GET;
import javax.ws.rs.POST;
import javax.ws.rs.Path;
import javax.ws.rs.PathParam;
import javax.ws.rs.Produces;
import javax.ws.rs.core.Response;
import javax.ws.rs.core.Response.Status;

@Path("/bookings")
public class BookingsREST {
   private BookingService bs = (BookingService)ServiceLocator.getService(BookingService.class);

   @POST
   @Consumes({"application/x-www-form-urlencoded"})
   @Path("/bookflights")
   @Produces({"application/json"})
   public Response bookFlights(@FormParam("userid") String userid, @FormParam("toFlightId") String toFlightId, @FormParam("toFlightSegId") String toFlightSegId, @FormParam("retFlightId") String retFlightId, @FormParam("retFlightSegId") String retFlightSegId, @FormParam("oneWayFlight") boolean oneWay) {
      try {
         BookingPK bookingIdTo = this.bs.bookFlight(userid, new FlightPK(toFlightSegId, toFlightId));
         BookingPK bookingIdReturn = null;
         if (!oneWay) {
            bookingIdReturn = this.bs.bookFlight(userid, new FlightPK(retFlightSegId, retFlightId));
         }

         BookingInfo bi;
         if (!oneWay) {
            bi = new BookingInfo(bookingIdTo.getId(), bookingIdReturn.getId(), oneWay);
         } else {
            bi = new BookingInfo(bookingIdTo.getId(), (String)null, oneWay);
         }

         return Response.ok(bi).build();
      } catch (Exception var10) {
         var10.printStackTrace();
         return Response.status(Status.INTERNAL_SERVER_ERROR).build();
      }
   }

   @GET
   @Path("/bybookingnumber/{userid}/{number}")
   @Produces({"application/json"})
   public Booking getBookingByNumber(@PathParam("number") String number, @FormParam("userid") String userid) {
      try {
         Booking b = this.bs.getBooking(userid, number);
         return b;
      } catch (Exception var4) {
         var4.printStackTrace();
         return null;
      }
   }

   @GET
   @Path("/byuser/{user}")
   @Produces({"application/json"})
   public List getBookingsByUser(@PathParam("user") String user) {
      try {
         return this.bs.getBookingsByUser(user);
      } catch (Exception var3) {
         var3.printStackTrace();
         return null;
      }
   }

   @POST
   @Consumes({"application/x-www-form-urlencoded"})
   @Path("/cancelbooking")
   @Produces({"application/json"})
   public Response cancelBookingsByNumber(@FormParam("number") String number, @FormParam("userid") String userid) {
      try {
         this.bs.cancelBooking(userid, number);
         return Response.ok("booking " + number + " deleted.").build();
      } catch (Exception var4) {
         var4.printStackTrace();
         return Response.status(Status.INTERNAL_SERVER_ERROR).build();
      }
   }
}
