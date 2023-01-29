#!/usr/bin/env python

from sys import path, argv
path.append('/home/net-lama/')

from modules.splib import MQTTClient
from getopt import getopt
from requests import post

argv = argv[1:]

try:
    opts, args = getopt(argv, "", [
        "clientId=",
        "clientType=",
        "mqttServer=", 
        "mqttPort=", 
        "dataTopic=", 
        "logTopic=",
        "hecServer=",
        "hecUrl=",
        "hecPort=",
        "hecToken="
        ])
except:
    print("Error")

for opt, arg in opts:
    if opt in ['--clientId']:
        clientId = arg
    elif opt in ['--clientType']:
        clientType = arg
    elif opt in ['--mqttServer']:
        mqttServer = arg
    elif opt in ['--mqttPort']:
        mqttPort = arg
    elif opt in ['--dataTopic']:
        dataTopic = arg
    elif opt in ['--logTopic']:
        logTopic = arg
    elif opt in ['--hecServer']:
        hecServer = arg
    elif opt in ['--hecUrl']:
        hecUrl = arg
    elif opt in ['--hecPort']:
        hecPort = arg
    elif opt in ['--hecToken']:
        hecToken = arg

clientInfo = {}
clientInfo['clientId'] = clientId
clientInfo['clientType'] = clientType
clientInfo['mqttServer'] = mqttServer
clientInfo['mqttPort'] = mqttPort
clientInfo['dataTopic'] = dataTopic
clientInfo['logTopic'] = logTopic
clientInfo['hecServer'] = hecServer
clientInfo['hecUrl'] = hecUrl
clientInfo['hecPort'] = hecPort
clientInfo['hecToken'] = hecToken

# Create MQTT client object
client = MQTTClient(**clientInfo)

# Initialise MQTT
client.create()

# Application specific functions
def sendData(dataQueue):
    splunkUrl = \
    'https://' + clientInfo['hecServer'] + ':' + \
    clientInfo['hecPort'] + clientInfo['hecUrl']
    authHeader = {"Authorization": f"Splunk {clientInfo['hecToken']}"}
    
    for data in dataQueue:
        event = {"host": "HEC-Forwarder", "event": data}
        print(event)
        req = post(splunkUrl, headers=authHeader, json=event, verify=False)
        client.send_log(f"Sending data to Splunk server: {req}")