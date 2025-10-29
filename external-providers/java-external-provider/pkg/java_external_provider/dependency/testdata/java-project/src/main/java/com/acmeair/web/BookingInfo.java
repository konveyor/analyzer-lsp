package com.acmeair.web;

public class BookingInfo {
   private String departBookingId;
   private String returnBookingId;
   private boolean oneWay;

   public BookingInfo(String departBookingId, String returnBookingId, boolean oneWay) {
      this.departBookingId = departBookingId;
      this.returnBookingId = returnBookingId;
      this.oneWay = oneWay;
   }

   public BookingInfo() {
   }

   public String getDepartBookingId() {
      return this.departBookingId;
   }

   public void setDepartBookingId(String departBookingId) {
      this.departBookingId = departBookingId;
   }

   public String getReturnBookingId() {
      return this.returnBookingId;
   }

   public void setReturnBookingId(String returnBookingId) {
      this.returnBookingId = returnBookingId;
   }

   public boolean isOneWay() {
      return this.oneWay;
   }

   public void setOneWay(boolean oneWay) {
      this.oneWay = oneWay;
   }

   public String toString() {
      return "BookingInfo [departBookingId=" + this.departBookingId + ", returnBookingId=" + this.returnBookingId + ", oneWay=" + this.oneWay + "]";
   }
}
