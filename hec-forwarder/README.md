# Splunk HEC forwarder
Reads data from the data topic and forwards it to the Splunk HEC endpoint. It collects a defined number of events and sends it as bulk to Splunk

Initial state: stop  
Config section:
```
"Hec-Forwarder": {
        "server": "10.140.70.1",
        "port": "8088",
        "url": "/services/collector",
        "token": "5c9e948b-7b06-41d7-8258-2be21cc1a884",
        "bulk": "1000"
    }
```