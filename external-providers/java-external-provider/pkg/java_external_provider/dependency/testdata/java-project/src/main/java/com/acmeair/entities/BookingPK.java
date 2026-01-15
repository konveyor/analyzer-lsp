package com.acmeair.entities;

import java.io.Serializable;
import javax.persistence.Column;
import javax.persistence.Embeddable;

@Embeddable
public class BookingPK implements Serializable {
   private static final long serialVersionUID = 1L;
   @Column(
      name = "bookingId"
   )
   private String id;
   private String customerId;

   public BookingPK() {
   }

   public BookingPK(String customerId, String id) {
      this.id = id;
      this.customerId = customerId;
   }

   public String getId() {
      return this.id;
   }

   public void setId(String id) {
      this.id = id;
   }

   public String getCustomerId() {
      return this.customerId;
   }

   public void setCustomerId(String customerId) {
      this.customerId = customerId;
   }

   public int hashCode() {
      int prime = true;
      int result = 1;
      result = 31 * result + (this.customerId == null ? 0 : this.customerId.hashCode());
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
         BookingPK other = (BookingPK)obj;
         if (this.customerId == null) {
            if (other.customerId != null) {
               return false;
            }
         } else if (!this.customerId.equals(other.customerId)) {
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
      return "BookingPK [customerId=" + this.customerId + ",id=" + this.id + "]";
   }
}
