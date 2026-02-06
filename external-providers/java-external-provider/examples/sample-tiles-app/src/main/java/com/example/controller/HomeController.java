package com.example.controller;

import com.example.model.B;
import com.example.service.HomeService;
import org.springframework.stereotype.Controller;
import org.springframework.ui.Model;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestMapping;

/**
 * Home controller that returns Tiles view names.
 */
@Controller
@RequestMapping("/")
public class HomeController {

    private HomeService homeService;

    public HomeController(HomeService homeService) {
        this.homeService = homeService;
    }

    public void doStuffWithHomeService() {
        B input = new B();
        homeService.doThings(input);
    }

    @GetMapping
    public String home(Model model) {
        model.addAttribute("message", "Welcome to our Spring 5 Tiles Application!");
        return "home";  // This resolves to a Tiles definition
    }

    @GetMapping("/about")
    public String about(Model model) {
        model.addAttribute("pageTitle", "About Us");
        return "about";  // This resolves to a Tiles definition
    }
}
