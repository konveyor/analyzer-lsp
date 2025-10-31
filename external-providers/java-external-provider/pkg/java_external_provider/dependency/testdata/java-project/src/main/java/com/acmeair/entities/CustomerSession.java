package com.acmeair.entities;

import java.io.Serializable;
import java.util.Date;
import javax.persistence.Column;
import javax.persistence.Entity;
import javax.persistence.Id;

@Entity
public class CustomerSession implements Serializable {
   private static final long serialVersionUID = 1L;
   @Id
   @Column(
      columnDefinition = "VARCHAR"
   )
   private String id;
   private String customerid;
   private Date lastAccessedTime;
   private Date timeoutTime;

   public CustomerSession() {
   }

   public CustomerSession(String id, String customerid, Date lastAccessedTime, Date timeoutTime) {
      this.id = id;
      this.customerid = customerid;
      this.lastAccessedTime = lastAccessedTime;
      this.timeoutTime = timeoutTime;
   }

   public String getId() {
      return this.id;
   }

   public void setId(String id) {
      this.id = id;
   }

   public String getCustomerid() {
      return this.customerid;
   }

   public void setCustomerid(String customerid) {
      this.customerid = customerid;
   }

   public Date getLastAccessedTime() {
      return this.lastAccessedTime;
   }

   public void setLastAccessedTime(Date lastAccessedTime) {
      this.lastAccessedTime = lastAccessedTime;
   }

   public Date getTimeoutTime() {
      return this.timeoutTime;
   }

   public void setTimeoutTime(Date timeoutTime) {
      this.timeoutTime = timeoutTime;
   }

   public String toString() {
      return "CustomerSession [id=" + this.id + ", customerid=" + this.customerid + ", lastAccessedTime=" + this.lastAccessedTime + ", timeoutTime=" + this.timeoutTime + "]";
   }

   public boolean equals(Object obj) {
      if (this == obj) {
         return true;
      } else if (obj == null) {
         return false;
      } else if (this.getClass() != obj.getClass()) {
         return false;
      } else {
         CustomerSession other = (CustomerSession)obj;
         if (this.customerid == null) {
            if (other.customerid != null) {
               return false;
            }
         } else if (!this.customerid.equals(other.customerid)) {
            return false;
         }

         if (this.id == null) {
            if (other.id != null) {
               return false;
            }
         } else if (!this.id.equals(other.id)) {
            return false;
         }

         if (this.lastAccessedTime == null) {
            if (other.lastAccessedTime != null) {
               return false;
            }
         } else if (!this.lastAccessedTime.equals(other.lastAccessedTime)) {
            return false;
         }

         if (this.timeoutTime == null) {
            if (other.timeoutTime != null) {
               return false;
            }
         } else if (!this.timeoutTime.equals(other.timeoutTime)) {
            return false;
         }

         return true;
      }
   }
}
