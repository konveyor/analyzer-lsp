package com.acmeair.web;

import java.util.List;

public class TripFlightOptions {
   private int tripLegs;
   private List tripFlights;

   public int getTripLegs() {
      return this.tripLegs;
   }

   public void setTripLegs(int tripLegs) {
      this.tripLegs = tripLegs;
   }

   public List getTripFlights() {
      return this.tripFlights;
   }

   public void setTripFlights(List tripFlights) {
      this.tripFlights = tripFlights;
   }
}
