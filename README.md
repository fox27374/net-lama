![Overview](https://github.com/fox27374/sensorpi/blob/master/overview.png)

# Net-Lama (still under development)
Intendet to be a flexible, modular environment to communicate between different client applications. Commands to the clients are done via MQTT. The clients itself update their status via REST API calls to the API endpoint. This way the whole state and capability information can be held on the central server which acts as API endpoint and webgui for user interaction.  
One example would be to have an Raspberry Pi with an external WLAN adapter so meassure WLAN parameters. The results are send to a MQTT topic where other clients can subscribe. In the first setup, there is an other client, that takes the data from the topic and send it to a Splunk HEC. Both clients can be controlled an monitored with the central net-lama server.  
The idea of the API endpoint is, that most of the clients can be deployed as docker containers without any configuration. The configuration itself is held on the API endpoint and clients can get their part of the configuration.

## ToDo
* Create webgui for configuration (flask)
* Implement error handling