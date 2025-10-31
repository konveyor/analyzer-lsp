package com.acmeair.entities;

import java.io.Serializable;
import javax.persistence.Embeddable;

@Embeddable
public class CustomerAddress implements Serializable {
   private static final long serialVersionUID = 1L;
   private String streetAddress1;
   private String streetAddress2;
   private String city;
   private String stateProvince;
   private String country;
   private String postalCode;

   public CustomerAddress() {
   }

   public CustomerAddress(String streetAddress1, String streetAddress2, String city, String stateProvince, String country, String postalCode) {
      this.streetAddress1 = streetAddress1;
      this.streetAddress2 = streetAddress2;
      this.city = city;
      this.stateProvince = stateProvince;
      this.country = country;
      this.postalCode = postalCode;
   }

   public String getStreetAddress1() {
      return this.streetAddress1;
   }

   public void setStreetAddress1(String streetAddress1) {
      this.streetAddress1 = streetAddress1;
   }

   public String getStreetAddress2() {
      return this.streetAddress2;
   }

   public void setStreetAddress2(String streetAddress2) {
      this.streetAddress2 = streetAddress2;
   }

   public String getCity() {
      return this.city;
   }

   public void setCity(String city) {
      this.city = city;
   }

   public String getStateProvince() {
      return this.stateProvince;
   }

   public void setStateProvince(String stateProvince) {
      this.stateProvince = stateProvince;
   }

   public String getCountry() {
      return this.country;
   }

   public void setCountry(String country) {
      this.country = country;
   }

   public String getPostalCode() {
      return this.postalCode;
   }

   public void setPostalCode(String postalCode) {
      this.postalCode = postalCode;
   }

   public String toString() {
      return "CustomerAddress [streetAddress1=" + this.streetAddress1 + ", streetAddress2=" + this.streetAddress2 + ", city=" + this.city + ", stateProvince=" + this.stateProvince + ", country=" + this.country + ", postalCode=" + this.postalCode + "]";
   }

   public boolean equals(Object obj) {
      if (this == obj) {
         return true;
      } else if (obj == null) {
         return false;
      } else if (this.getClass() != obj.getClass()) {
         return false;
      } else {
         CustomerAddress other = (CustomerAddress)obj;
         if (this.city == null) {
            if (other.city != null) {
               return false;
            }
         } else if (!this.city.equals(other.city)) {
            return false;
         }

         if (this.country == null) {
            if (other.country != null) {
               return false;
            }
         } else if (!this.country.equals(other.country)) {
            return false;
         }

         if (this.postalCode == null) {
            if (other.postalCode != null) {
               return false;
            }
         } else if (!this.postalCode.equals(other.postalCode)) {
            return false;
         }

         if (this.stateProvince == null) {
            if (other.stateProvince != null) {
               return false;
            }
         } else if (!this.stateProvince.equals(other.stateProvince)) {
            return false;
         }

         if (this.streetAddress1 == null) {
            if (other.streetAddress1 != null) {
               return false;
            }
         } else if (!this.streetAddress1.equals(other.streetAddress1)) {
            return false;
         }

         if (this.streetAddress2 == null) {
            if (other.streetAddress2 != null) {
               return false;
            }
         } else if (!this.streetAddress2.equals(other.streetAddress2)) {
            return false;
         }

         return true;
      }
   }
}
