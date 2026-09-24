package com.example.itemsapi;

import java.util.Map;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
public class HelloController {

    // The one real endpoint: what the frontend's "Say hello" button calls,
    // through nginx's same-origin proxy (frontend/nginx.conf) rather
    // than directly -- see that file for why that means no CORS config is
    // needed here at all.
    @GetMapping("/api/hello")
    public Map<String, String> hello() {
        return Map.of("message", "Hello from Backend");
    }

    // What the Dockerfile's HEALTHCHECK and the k8s startup/readiness/
    // liveness probes all poll -- any 200 response is enough for both.
    @GetMapping("/healthz")
    public String healthz() {
        return "OK";
    }
}
