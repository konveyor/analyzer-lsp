package com.acmeair.entities;

import java.io.Serializable;
import javax.persistence.Entity;
import javax.persistence.Id;

@Entity
public class AirportCodeMapping implements Serializable {
   private static final long serialVersionUID = 1L;
   @Id
   private String id;
   private String airportName;

   public AirportCodeMapping() {
   }

   public AirportCodeMapping(String airportCode, String airportName) {
      this.id = airportCode;
      this.airportName = airportName;
   }

   public String getAirportCode() {
      return this.id;
   }

   public void setAirportCode(String airportCode) {
      this.id = airportCode;
   }

   public String getAirportName() {
      return this.airportName;
   }

   public void setAirportName(String airportName) {
      this.airportName = airportName;
   }
}
