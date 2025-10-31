package com.acmeair.entities;

import java.io.Serializable;
import java.math.BigDecimal;
import java.util.Date;
import javax.persistence.EmbeddedId;
import javax.persistence.Entity;

@Entity
public class Flight implements Serializable {
   private static final long serialVersionUID = 1L;
   @EmbeddedId
   private FlightPK pkey;
   private Date scheduledDepartureTime;
   private Date scheduledArrivalTime;
   private BigDecimal firstClassBaseCost;
   private BigDecimal economyClassBaseCost;
   private int numFirstClassSeats;
   private int numEconomyClassSeats;
   private String airplaneTypeId;
   private FlightSegment flightSegment;

   public Flight() {
   }

   public Flight(String id, String flightSegmentId, Date scheduledDepartureTime, Date scheduledArrivalTime, BigDecimal firstClassBaseCost, BigDecimal economyClassBaseCost, int numFirstClassSeats, int numEconomyClassSeats, String airplaneTypeId) {
      this.pkey = new FlightPK(flightSegmentId, id);
      this.scheduledDepartureTime = scheduledDepartureTime;
      this.scheduledArrivalTime = scheduledArrivalTime;
      this.firstClassBaseCost = firstClassBaseCost;
      this.economyClassBaseCost = economyClassBaseCost;
      this.numFirstClassSeats = numFirstClassSeats;
      this.numEconomyClassSeats = numEconomyClassSeats;
      this.airplaneTypeId = airplaneTypeId;
   }

   public FlightPK getPkey() {
      return this.pkey;
   }

   public void setPkey(FlightPK pkey) {
      this.pkey = pkey;
   }

   public String getFlightSegmentId() {
      return this.pkey.getFlightSegmentId();
   }

   public Date getScheduledDepartureTime() {
      return this.scheduledDepartureTime;
   }

   public void setScheduledDepartureTime(Date scheduledDepartureTime) {
      this.scheduledDepartureTime = scheduledDepartureTime;
   }

   public Date getScheduledArrivalTime() {
      return this.scheduledArrivalTime;
   }

   public void setScheduledArrivalTime(Date scheduledArrivalTime) {
      this.scheduledArrivalTime = scheduledArrivalTime;
   }

   public BigDecimal getFirstClassBaseCost() {
      return this.firstClassBaseCost;
   }

   public void setFirstClassBaseCost(BigDecimal firstClassBaseCost) {
      this.firstClassBaseCost = firstClassBaseCost;
   }

   public BigDecimal getEconomyClassBaseCost() {
      return this.economyClassBaseCost;
   }

   public void setEconomyClassBaseCost(BigDecimal economyClassBaseCost) {
      this.economyClassBaseCost = economyClassBaseCost;
   }

   public int getNumFirstClassSeats() {
      return this.numFirstClassSeats;
   }

   public void setNumFirstClassSeats(int numFirstClassSeats) {
      this.numFirstClassSeats = numFirstClassSeats;
   }

   public int getNumEconomyClassSeats() {
      return this.numEconomyClassSeats;
   }

   public void setNumEconomyClassSeats(int numEconomyClassSeats) {
      this.numEconomyClassSeats = numEconomyClassSeats;
   }

   public String getAirplaneTypeId() {
      return this.airplaneTypeId;
   }

   public void setAirplaneTypeId(String airplaneTypeId) {
      this.airplaneTypeId = airplaneTypeId;
   }

   public FlightSegment getFlightSegment() {
      return this.flightSegment;
   }

   public void setFlightSegment(FlightSegment flightSegment) {
      this.flightSegment = flightSegment;
   }

   public String toString() {
      return "Flight key=" + this.pkey + ", scheduledDepartureTime=" + this.scheduledDepartureTime + ", scheduledArrivalTime=" + this.scheduledArrivalTime + ", firstClassBaseCost=" + this.firstClassBaseCost + ", economyClassBaseCost=" + this.economyClassBaseCost + ", numFirstClassSeats=" + this.numFirstClassSeats + ", numEconomyClassSeats=" + this.numEconomyClassSeats + ", airplaneTypeId=" + this.airplaneTypeId + "]";
   }

   public boolean equals(Object obj) {
      if (this == obj) {
         return true;
      } else if (obj == null) {
         return false;
      } else if (this.getClass() != obj.getClass()) {
         return false;
      } else {
         Flight other = (Flight)obj;
         if (this.airplaneTypeId == null) {
            if (other.airplaneTypeId != null) {
               return false;
            }
         } else if (!this.airplaneTypeId.equals(other.airplaneTypeId)) {
            return false;
         }

         if (this.economyClassBaseCost == null) {
            if (other.economyClassBaseCost != null) {
               return false;
            }
         } else if (!this.economyClassBaseCost.equals(other.economyClassBaseCost)) {
            return false;
         }

         if (this.firstClassBaseCost == null) {
            if (other.firstClassBaseCost != null) {
               return false;
            }
         } else if (!this.firstClassBaseCost.equals(other.firstClassBaseCost)) {
            return false;
         }

         if (this.flightSegment == null) {
            if (other.flightSegment != null) {
               return false;
            }
         } else if (!this.flightSegment.equals(other.flightSegment)) {
            return false;
         }

         if (this.pkey == null) {
            if (other.pkey != null) {
               return false;
            }
         } else if (!this.pkey.equals(other.pkey)) {
            return false;
         }

         if (this.numEconomyClassSeats != other.numEconomyClassSeats) {
            return false;
         } else if (this.numFirstClassSeats != other.numFirstClassSeats) {
            return false;
         } else {
            if (this.scheduledArrivalTime == null) {
               if (other.scheduledArrivalTime != null) {
                  return false;
               }
            } else if (!this.scheduledArrivalTime.equals(other.scheduledArrivalTime)) {
               return false;
            }

            if (this.scheduledDepartureTime == null) {
               if (other.scheduledDepartureTime != null) {
                  return false;
               }
            } else if (!this.scheduledDepartureTime.equals(other.scheduledDepartureTime)) {
               return false;
            }

            return true;
         }
      }
   }
}
