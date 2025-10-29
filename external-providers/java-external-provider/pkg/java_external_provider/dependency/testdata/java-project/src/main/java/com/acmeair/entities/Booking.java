package com.acmeair.entities;

import java.io.Serializable;
import java.util.Date;
import javax.persistence.EmbeddedId;
import javax.persistence.Entity;
import javax.persistence.ManyToOne;
import javax.persistence.PrimaryKeyJoinColumn;

@Entity
public class Booking implements Serializable {
   private static final long serialVersionUID = 1L;
   @EmbeddedId
   private BookingPK pkey;
   private FlightPK flightKey;
   private Date dateOfBooking;
   @ManyToOne
   @PrimaryKeyJoinColumn(
      name = "customerId",
      referencedColumnName = "id"
   )
   private Customer customer;
   private Flight flight;

   public Booking() {
   }

   public Booking(String id, Date dateOfFlight, Customer customer, Flight flight) {
      this.pkey = new BookingPK(customer.getUsername(), id);
      this.flightKey = flight.getPkey();
      this.dateOfBooking = dateOfFlight;
      this.customer = customer;
      this.flight = flight;
   }

   public BookingPK getPkey() {
      return this.pkey;
   }

   public String getCustomerId() {
      return this.pkey.getCustomerId();
   }

   public void setPkey(BookingPK pkey) {
      this.pkey = pkey;
   }

   public FlightPK getFlightKey() {
      return this.flightKey;
   }

   public void setFlightKey(FlightPK flightKey) {
      this.flightKey = flightKey;
   }

   public void setFlight(Flight flight) {
      this.flight = flight;
   }

   public Date getDateOfBooking() {
      return this.dateOfBooking;
   }

   public void setDateOfBooking(Date dateOfBooking) {
      this.dateOfBooking = dateOfBooking;
   }

   public Customer getCustomer() {
      return this.customer;
   }

   public Flight getFlight() {
      return this.flight;
   }

   public String toString() {
      return "Booking [key=" + this.pkey + ", flightKey=" + this.flightKey + ", dateOfBooking=" + this.dateOfBooking + ", customer=" + this.customer + ", flight=" + this.flight + "]";
   }

   public boolean equals(Object obj) {
      if (this == obj) {
         return true;
      } else if (obj == null) {
         return false;
      } else if (this.getClass() != obj.getClass()) {
         return false;
      } else {
         Booking other = (Booking)obj;
         if (this.customer == null) {
            if (other.customer != null) {
               return false;
            }
         } else if (!this.customer.equals(other.customer)) {
            return false;
         }

         if (this.dateOfBooking == null) {
            if (other.dateOfBooking != null) {
               return false;
            }
         } else if (!this.dateOfBooking.equals(other.dateOfBooking)) {
            return false;
         }

         if (this.flight == null) {
            if (other.flight != null) {
               return false;
            }
         } else if (!this.flight.equals(other.flight)) {
            return false;
         }

         if (this.flightKey == null) {
            if (other.flightKey != null) {
               return false;
            }
         } else if (!this.flightKey.equals(other.flightKey)) {
            return false;
         }

         if (this.pkey == null) {
            if (other.pkey != null) {
               return false;
            }
         } else if (!this.pkey.equals(other.pkey)) {
            return false;
         }

         return true;
      }
   }
}
