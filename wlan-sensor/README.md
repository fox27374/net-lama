# WLAN sensor
It comes with two modes: scan and sensor.  
The scan mode scans nearby WLANs and writes infos to the application config section on the API endpoint.  
The sensor mode collects all packets from a specific BSSID and sends it to the MQTT data topic.  
As the sensor requires the WLAN adapter to be in promiscuous mode, the application needs to be natively installed and is currently not docker compatible. Also som preparations need to be done beforehand, for example compiling the correct WLAN driver and changing the access rights to /sbin/ifconfig binary, as the application needs to change the WLAN channels.  

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