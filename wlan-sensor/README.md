# WLAN sensor
It comes with two modes: scan and sensor.  
The scan mode scans nearby WLANs and writes infos to the application config section on the API endpoint.  
The sensor mode collects all packets from a specific BSSID and sends it to the MQTT data topic.  

Initial state: stop  
Config section:
```
"WlanSensor": {
        "interface": "wlan1",
        "scanChannels": [
            1,
            6,
            11
        ],
        "scanTime": "5",
        "sensorChannel": "1",
        "wlans": {}
    }
```