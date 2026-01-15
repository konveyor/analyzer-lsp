package com.acmeair.entities;

import java.io.Serializable;
import javax.persistence.Column;
import javax.persistence.Embeddable;

@Embeddable
public class FlightPK implements Serializable {
   private static final long serialVersionUID = 1L;
   @Column(
      name = "flightId"
   )
   private String id;
   private String flightSegmentId;

   public FlightPK() {
   }

   public FlightPK(String flightSegmentId, String id) {
      this.id = id;
      this.flightSegmentId = flightSegmentId;
   }

   public String getId() {
      return this.id;
   }

   public void setId(String id) {
      this.id = id;
   }

   public String getFlightSegmentId() {
      return this.flightSegmentId;
   }

   public void setFlightSegmentId(String flightSegmentId) {
      this.flightSegmentId = flightSegmentId;
   }

   public int hashCode() {
      int prime = true;
      int result = 1;
      result = 31 * result + (this.flightSegmentId == null ? 0 : this.flightSegmentId.hashCode());
      result = 31 * result + (this.id == null ? 0 : this.id.hashCode());
      return result;
   }

   public boolean equals(Object obj) {
      if (this == obj) {
         return true;
      } else if (obj == null) {
         return false;
      } else if (this.getClass() != obj.getClass()) {
         return false;
      } else {
         FlightPK other = (FlightPK)obj;
         if (this.flightSegmentId == null) {
            if (other.flightSegmentId != null) {
               return false;
            }
         } else if (!this.flightSegmentId.equals(other.flightSegmentId)) {
            return false;
         }

         if (this.id == null) {
            if (other.id != null) {
               return false;
            }
         } else if (!this.id.equals(other.id)) {
            return false;
         }

         return true;
      }
   }

   public String toString() {
      return "FlightPK [flightSegmentId=" + this.flightSegmentId + ",id=" + this.id + "]";
   }
}
