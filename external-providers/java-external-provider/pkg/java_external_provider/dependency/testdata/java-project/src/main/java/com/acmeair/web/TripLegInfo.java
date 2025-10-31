package com.acmeair.web;

import java.util.List;

public class TripLegInfo {
   public static int DEFAULT_PAGE_SIZE = 10;
   private boolean hasMoreOptions;
   private int numPages;
   private int pageSize;
   private int currentPage;
   private List flightsOptions;

   public boolean isHasMoreOptions() {
      return this.hasMoreOptions;
   }

   public void setHasMoreOptions(boolean hasMoreOptions) {
      this.hasMoreOptions = hasMoreOptions;
   }

   public int getNumPages() {
      return this.numPages;
   }

   public void setNumPages(int numPages) {
      this.numPages = numPages;
   }

   public int getPageSize() {
      return this.pageSize;
   }

   public void setPageSize(int pageSize) {
      this.pageSize = pageSize;
   }

   public int getCurrentPage() {
      return this.currentPage;
   }

   public void setCurrentPage(int currentPage) {
      this.currentPage = currentPage;
   }

   public List getFlightsOptions() {
      return this.flightsOptions;
   }

   public void setFlightsOptions(List flightsOptions) {
      this.flightsOptions = flightsOptions;
   }
}
