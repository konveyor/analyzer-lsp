package com.acmeair.web;

import com.acmeair.service.FlightService;
import java.util.ArrayList;
import java.util.Date;
import java.util.List;
import javax.ws.rs.Consumes;
import javax.ws.rs.FormParam;
import javax.ws.rs.POST;
import javax.ws.rs.Path;
import javax.ws.rs.Produces;

@Path("/flights")
public class FlightsREST {
   private FlightService flightService = (FlightService)ServiceLocator.getService(FlightService.class);

   @POST
   @Path("/queryflights")
   @Consumes({"application/x-www-form-urlencoded"})
   @Produces({"application/json"})
   public TripFlightOptions getTripFlights(@FormParam("fromAirport") String fromAirport, @FormParam("toAirport") String toAirport, @FormParam("fromDate") Date fromDate, @FormParam("returnDate") Date returnDate, @FormParam("oneWay") boolean oneWay) {
      TripFlightOptions options = new TripFlightOptions();
      ArrayList legs = new ArrayList();
      TripLegInfo toInfo = new TripLegInfo();
      List toFlights = this.flightService.getFlightByAirportsAndDepartureDate(fromAirport, toAirport, fromDate);
      toInfo.setFlightsOptions(toFlights);
      legs.add(toInfo);
      toInfo.setCurrentPage(0);
      toInfo.setHasMoreOptions(false);
      toInfo.setNumPages(1);
      toInfo.setPageSize(TripLegInfo.DEFAULT_PAGE_SIZE);
      if (!oneWay) {
         TripLegInfo retInfo = new TripLegInfo();
         List retFlights = this.flightService.getFlightByAirportsAndDepartureDate(toAirport, fromAirport, returnDate);
         retInfo.setFlightsOptions(retFlights);
         legs.add(retInfo);
         retInfo.setCurrentPage(0);
         retInfo.setHasMoreOptions(false);
         retInfo.setNumPages(1);
         retInfo.setPageSize(TripLegInfo.DEFAULT_PAGE_SIZE);
         options.setTripLegs(2);
      } else {
         options.setTripLegs(1);
      }

      options.setTripFlights(legs);
      return options;
   }

   @POST
   @Path("/browseflights")
   @Consumes({"application/x-www-form-urlencoded"})
   @Produces({"application/json"})
   public TripFlightOptions browseFlights(@FormParam("fromAirport") String fromAirport, @FormParam("toAirport") String toAirport, @FormParam("oneWay") boolean oneWay) {
      TripFlightOptions options = new TripFlightOptions();
      ArrayList legs = new ArrayList();
      TripLegInfo toInfo = new TripLegInfo();
      List toFlights = this.flightService.getFlightByAirports(fromAirport, toAirport);
      toInfo.setFlightsOptions(toFlights);
      legs.add(toInfo);
      toInfo.setCurrentPage(0);
      toInfo.setHasMoreOptions(false);
      toInfo.setNumPages(1);
      toInfo.setPageSize(TripLegInfo.DEFAULT_PAGE_SIZE);
      if (!oneWay) {
         TripLegInfo retInfo = new TripLegInfo();
         List retFlights = this.flightService.getFlightByAirports(toAirport, fromAirport);
         retInfo.setFlightsOptions(retFlights);
         legs.add(retInfo);
         retInfo.setCurrentPage(0);
         retInfo.setHasMoreOptions(false);
         retInfo.setNumPages(1);
         retInfo.setPageSize(TripLegInfo.DEFAULT_PAGE_SIZE);
         options.setTripLegs(2);
      } else {
         options.setTripLegs(1);
      }

      options.setTripFlights(legs);
      return options;
   }
}
