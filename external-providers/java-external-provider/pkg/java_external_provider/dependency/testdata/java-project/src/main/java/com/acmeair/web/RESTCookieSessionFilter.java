package com.acmeair.web;

import com.acmeair.entities.CustomerSession;
import com.acmeair.service.CustomerService;
import java.io.IOException;
import javax.annotation.Resource;
import javax.servlet.Filter;
import javax.servlet.FilterChain;
import javax.servlet.FilterConfig;
import javax.servlet.ServletException;
import javax.servlet.ServletRequest;
import javax.servlet.ServletResponse;
import javax.servlet.http.Cookie;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;
import javax.sql.DataSource;

public class RESTCookieSessionFilter implements Filter {
   static final String LOGIN_USER = "acmeair.login_user";
   private static final String LOGIN_PATH = "/rest/api/login";
   private static final String LOGOUT_PATH = "/rest/api/login/logout";
   private CustomerService customerService = (CustomerService)ServiceLocator.getService(CustomerService.class);
   @Resource(
      name = "jdbc/acmeairdatasource"
   )
   DataSource source1;

   public void destroy() {
   }

   public void doFilter(ServletRequest req, ServletResponse resp, FilterChain chain) throws IOException, ServletException {
      HttpServletRequest request = (HttpServletRequest)req;
      HttpServletResponse response = (HttpServletResponse)resp;
      String path = request.getServletPath() + request.getPathInfo();
      if (!path.endsWith("/rest/api/login") && !path.endsWith("/rest/api/login/logout") && !path.startsWith("/rest/api/loader/")) {
         Cookie[] cookies = request.getCookies();
         Cookie sessionCookie = null;
         if (cookies == null) {
            response.sendError(403);
         } else {
            Cookie[] var9 = cookies;
            int var10 = cookies.length;

            for(int var11 = 0; var11 < var10; ++var11) {
               Cookie c = var9[var11];
               if (c.getName().equals(LoginREST.SESSIONID_COOKIE_NAME)) {
                  sessionCookie = c;
               }

               if (sessionCookie != null) {
                  break;
               }
            }

            String sessionId = "";
            if (sessionCookie != null) {
               sessionId = sessionCookie.getValue().trim();
            }

            if (sessionId.equals("")) {
               response.sendError(403);
            } else {
               CustomerSession cs = this.customerService.validateSession(sessionId);
               if (cs != null) {
                  request.setAttribute("acmeair.login_user", cs.getCustomerid());
                  chain.doFilter(req, resp);
               } else {
                  response.sendError(403);
               }
            }
         }
      } else {
         chain.doFilter(req, resp);
      }
   }

   public void init(FilterConfig config) throws ServletException {
   }
}
