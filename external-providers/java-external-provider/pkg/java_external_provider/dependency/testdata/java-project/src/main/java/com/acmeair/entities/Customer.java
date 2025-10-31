package com.acmeair.entities;

import java.io.Serializable;
import javax.persistence.Column;
import javax.persistence.Embedded;
import javax.persistence.Entity;
import javax.persistence.Id;

@Entity
public class Customer implements Serializable {
   private static final long serialVersionUID = 1L;
   @Id
   @Column(
      columnDefinition = "VARCHAR"
   )
   private String id;
   private String password;
   private MemberShipStatus status;
   private int total_miles;
   private int miles_ytd;
   @Embedded
   private CustomerAddress address;
   private String phoneNumber;
   private PhoneType phoneNumberType;

   public Customer() {
   }

   public Customer(String username, String password, MemberShipStatus status, int total_miles, int miles_ytd, CustomerAddress address, String phoneNumber, PhoneType phoneNumberType) {
      this.id = username;
      this.password = password;
      this.status = status;
      this.total_miles = total_miles;
      this.miles_ytd = miles_ytd;
      this.address = address;
      this.phoneNumber = phoneNumber;
      this.phoneNumberType = phoneNumberType;
   }

   public String getUsername() {
      return this.id;
   }

   public void setUsername(String username) {
      this.id = username;
   }

   public String getPassword() {
      return this.password;
   }

   public void setPassword(String password) {
      this.password = password;
   }

   public MemberShipStatus getStatus() {
      return this.status;
   }

   public void setStatus(MemberShipStatus status) {
      this.status = status;
   }

   public int getTotal_miles() {
      return this.total_miles;
   }

   public void setTotal_miles(int total_miles) {
      this.total_miles = total_miles;
   }

   public int getMiles_ytd() {
      return this.miles_ytd;
   }

   public void setMiles_ytd(int miles_ytd) {
      this.miles_ytd = miles_ytd;
   }

   public String getPhoneNumber() {
      return this.phoneNumber;
   }

   public void setPhoneNumber(String phoneNumber) {
      this.phoneNumber = phoneNumber;
   }

   public PhoneType getPhoneNumberType() {
      return this.phoneNumberType;
   }

   public void setPhoneNumberType(PhoneType phoneNumberType) {
      this.phoneNumberType = phoneNumberType;
   }

   public CustomerAddress getAddress() {
      return this.address;
   }

   public void setAddress(CustomerAddress address) {
      this.address = address;
   }

   public String toString() {
      return "Customer [id=" + this.id + ", password=" + this.password + ", status=" + this.status + ", total_miles=" + this.total_miles + ", miles_ytd=" + this.miles_ytd + ", address=" + this.address + ", phoneNumber=" + this.phoneNumber + ", phoneNumberType=" + this.phoneNumberType + "]";
   }

   public boolean equals(Object obj) {
      if (this == obj) {
         return true;
      } else if (obj == null) {
         return false;
      } else if (this.getClass() != obj.getClass()) {
         return false;
      } else {
         Customer other = (Customer)obj;
         if (this.address == null) {
            if (other.address != null) {
               return false;
            }
         } else if (!this.address.equals(other.address)) {
            return false;
         }

         if (this.id == null) {
            if (other.id != null) {
               return false;
            }
         } else if (!this.id.equals(other.id)) {
            return false;
         }

         if (this.miles_ytd != other.miles_ytd) {
            return false;
         } else {
            if (this.password == null) {
               if (other.password != null) {
                  return false;
               }
            } else if (!this.password.equals(other.password)) {
               return false;
            }

            if (this.phoneNumber == null) {
               if (other.phoneNumber != null) {
                  return false;
               }
            } else if (!this.phoneNumber.equals(other.phoneNumber)) {
               return false;
            }

            if (this.phoneNumberType != other.phoneNumberType) {
               return false;
            } else if (this.status != other.status) {
               return false;
            } else {
               return this.total_miles == other.total_miles;
            }
         }
      }
   }

   public static enum PhoneType {
      UNKNOWN,
      HOME,
      BUSINESS,
      MOBILE;
   }

   public static enum MemberShipStatus {
      NONE,
      SILVER,
      GOLD,
      PLATINUM,
      EXEC_PLATINUM,
      GRAPHITE;
   }
}
