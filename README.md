# Headers-by-request

For each incoming Treafik route request, a Post request is send to the `urlHeaderRequest` endpoint. 
Which should response with a list of headers to apply to the original Traefik request. When the `url`

Configuration:
```yaml
plugin:
  headers-by-request:
    urlHeaderRequest: "http://127.0.0.1/resolve"
    enableTiming: true
```

Request:
```json
{ 
  "request": "https://route-to-app.domain.com/volume1"
}
```

Response:
```json
{ 
  "headers": {
    "header1": "value1",
    "header2": "value2"
    }
}
```