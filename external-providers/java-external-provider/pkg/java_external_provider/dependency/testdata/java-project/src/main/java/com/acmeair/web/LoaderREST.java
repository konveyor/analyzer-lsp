package com.acmeair.web;

import com.acmeair.entities.AirportCodeMapping;
import com.acmeair.entities.Customer;
import com.acmeair.entities.CustomerAddress;
import com.acmeair.entities.FlightSegment;
import com.acmeair.entities.Customer.MemberShipStatus;
import com.acmeair.entities.Customer.PhoneType;
import com.acmeair.service.CustomerService;
import com.acmeair.service.FlightService;
import java.io.InputStream;
import java.io.InputStreamReader;
import java.io.LineNumberReader;
import java.math.BigDecimal;
import java.util.ArrayList;
import java.util.Calendar;
import java.util.Date;
import java.util.StringTokenizer;
import javax.ws.rs.GET;
import javax.ws.rs.Path;
import javax.ws.rs.Produces;
import org.springframework.stereotype.Component;

@Path("/loader")
@Component
public class LoaderREST {
   private CustomerService customerService = (CustomerService)ServiceLocator.getService(CustomerService.class);
   private FlightService flightService = (FlightService)ServiceLocator.getService(FlightService.class);
   private static Object lock = new Object();

   @GET
   @Path("/load")
   @Produces({"text/plain"})
   public String load() {
      return this.loadData(10L, 30);
   }

   @GET
   @Path("/loadSmall")
   @Produces({"text/plain"})
   public String loadSmall() {
      return this.loadData(5L, 5);
   }

   @GET
   @Path("/loadTiny")
   @Produces({"text/plain"})
   public String loadTiny() {
      return this.loadData(2L, 2);
   }

   private String loadData(long numCustomers, int segments) {
      synchronized(lock) {
         try {
            this.loadCustomers(numCustomers);
         } catch (Exception var8) {
            var8.printStackTrace();
         }

         try {
            this.loadFlights(segments);
         } catch (Exception var7) {
            var7.printStackTrace();
         }

         return "Sample data loaded.";
      }
   }

   public void loadCustomers(long numCustomers) {
      System.out.println("Loading customer data...");
      CustomerAddress address = new CustomerAddress("123 Main St.", (String)null, "Anytown", "NC", "USA", "27617");

      for(long ii = 0L; ii < numCustomers; ++ii) {
         String id = "uid" + ii + "@email.com";
         Customer customer = this.customerService.getCustomerByUsername(id);
         if (customer == null) {
            this.customerService.createCustomer(id, "password", MemberShipStatus.GOLD, 1000000, 1000, "919-123-4567", PhoneType.BUSINESS, address);
         }
      }

      System.out.println("Done loading customer data.");
   }

   public void loadFlights(int segments) throws Exception {
      System.out.println("Loading flight data...");
      InputStream csvInputStream = this.getClass().getResourceAsStream("/mileage.csv");
      LineNumberReader lnr = new LineNumberReader(new InputStreamReader(csvInputStream));
      String line1 = lnr.readLine();
      StringTokenizer st = new StringTokenizer(line1, ",");
      ArrayList airports = new ArrayList();

      while(st.hasMoreTokens()) {
         AirportCodeMapping acm = new AirportCodeMapping();
         acm.setAirportName(st.nextToken());
         airports.add(acm);
      }

      String line2 = lnr.readLine();
      st = new StringTokenizer(line2, ",");

      String line;
      for(int ii = 0; st.hasMoreTokens(); ++ii) {
         line = st.nextToken();
         ((AirportCodeMapping)airports.get(ii)).setAirportCode(line);
      }

      int flightNumber = 0;

      label61:
      while(true) {
         line = lnr.readLine();
         if (line == null || line.trim().equals("")) {
            for(int jj = 0; jj < airports.size(); ++jj) {
               this.flightService.storeAirportMapping((AirportCodeMapping)airports.get(jj));
            }

            lnr.close();
            System.out.println("Done loading flight data.");
            return;
         }

         st = new StringTokenizer(line, ",");
         String airportName = st.nextToken();
         String airportCode = st.nextToken();
         if (!alreadyInCollection(airportCode, airports)) {
            AirportCodeMapping acm = new AirportCodeMapping();
            acm.setAirportName(airportName);
            acm.setAirportCode(airportCode);
            airports.add(acm);
         }

         int indexIntoTopLine = 0;

         while(true) {
            while(true) {
               if (!st.hasMoreTokens()) {
                  continue label61;
               }

               String milesString = st.nextToken();
               if (milesString.equals("NA")) {
                  ++indexIntoTopLine;
               } else {
                  int miles = Integer.parseInt(milesString);
                  String toAirport = ((AirportCodeMapping)airports.get(indexIntoTopLine)).getAirportCode();
                  if (this.flightService.getFlightByAirports(airportCode, toAirport).isEmpty()) {
                     String flightId = "AA" + flightNumber;
                     FlightSegment flightSeg = new FlightSegment(flightId, airportCode, toAirport, miles);
                     this.flightService.storeFlightSegment(flightSeg);
                     Date now = new Date();

                     for(int daysFromNow = 0; daysFromNow < segments; ++daysFromNow) {
                        Calendar c = Calendar.getInstance();
                        c.setTime(now);
                        c.set(11, 0);
                        c.set(12, 0);
                        c.set(13, 0);
                        c.set(14, 0);
                        c.add(5, daysFromNow);
                        Date departureTime = c.getTime();
                        Date arrivalTime = getArrivalTime(departureTime, miles);
                        this.flightService.createNewFlight(flightId, departureTime, arrivalTime, new BigDecimal(500), new BigDecimal(200), 10, 200, "B747");
                     }

                     ++flightNumber;
                     ++indexIntoTopLine;
                  }
               }
            }
         }
      }
   }

   private static Date getArrivalTime(Date departureTime, int mileage) {
      double averageSpeed = 600.0;
      double hours = (double)mileage / averageSpeed;
      double partsOfHour = hours % 1.0;
      int minutes = (int)(60.0 * partsOfHour);
      Calendar c = Calendar.getInstance();
      c.setTime(departureTime);
      c.add(10, (int)hours);
      c.add(12, minutes);
      return c.getTime();
   }

   private static boolean alreadyInCollection(String airportCode, ArrayList airports) {
      for(int ii = 0; ii < airports.size(); ++ii) {
         if (((AirportCodeMapping)airports.get(ii)).getAirportCode().equals(airportCode)) {
            return true;
         }
      }

      return false;
   }
}
