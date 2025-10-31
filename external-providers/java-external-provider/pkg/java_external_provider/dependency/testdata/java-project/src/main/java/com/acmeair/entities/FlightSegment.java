package com.acmeair.entities;

import java.io.Serializable;
import javax.persistence.Entity;
import javax.persistence.Id;

@Entity
public class FlightSegment implements Serializable {
   private static final long serialVersionUID = 1L;
   @Id
   private String id;
   private String originPort;
   private String destPort;
   private int miles;

   public FlightSegment() {
   }

   public FlightSegment(String flightName, String origPort, String destPort, int miles) {
      this.id = flightName;
      this.originPort = origPort;
      this.destPort = destPort;
      this.miles = miles;
   }

   public String getFlightName() {
      return this.id;
   }

   public void setFlightName(String flightName) {
      this.id = flightName;
   }

   public String getOriginPort() {
      return this.originPort;
   }

   public void setOriginPort(String originPort) {
      this.originPort = originPort;
   }

   public String getDestPort() {
      return this.destPort;
   }

   public void setDestPort(String destPort) {
      this.destPort = destPort;
   }

   public int getMiles() {
      return this.miles;
   }

   public void setMiles(int miles) {
      this.miles = miles;
   }

   public String toString() {
      StringBuffer sb = new StringBuffer();
      sb.append("FlightSegment ").append(this.id).append(" originating from:\"").append(this.originPort).append("\" arriving at:\"").append(this.destPort).append("\"");
      return sb.toString();
   }

   public boolean equals(Object obj) {
      if (this == obj) {
         return true;
      } else if (obj == null) {
         return false;
      } else if (this.getClass() != obj.getClass()) {
         return false;
      } else {
         FlightSegment other = (FlightSegment)obj;
         if (this.destPort == null) {
            if (other.destPort != null) {
               return false;
            }
         } else if (!this.destPort.equals(other.destPort)) {
            return false;
         }

         if (this.id == null) {
            if (other.id != null) {
               return false;
            }
         } else if (!this.id.equals(other.id)) {
            return false;
         }

         if (this.miles != other.miles) {
            return false;
         } else {
            if (this.originPort == null) {
               if (other.originPort != null) {
                  return false;
               }
            } else if (!this.originPort.equals(other.originPort)) {
               return false;
            }

            return true;
         }
      }
   }
}
