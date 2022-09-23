#!/usr/bin/env python

from sys import path, argv
path.append('/home/net-lama/')

from modules.splib import MQTTClient
from speedtest import Speedtest
from getopt import getopt

argv = argv[1:]

try:
    opts, args = getopt(argv, "", [
        "clientId=",
        "clientType=",
        "mqttServer=", 
        "mqttPort=", 
        "dataTopic=", 
        "logTopic="
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

clientInfo = {}
clientInfo['clientId'] = clientId
clientInfo['clientType'] = clientType
clientInfo['mqttServer'] = mqttServer
clientInfo['mqttPort'] = mqttPort
clientInfo['dataTopic'] = dataTopic
clientInfo['logTopic'] = logTopic

# Create MQTT client object
client = MQTTClient(**clientInfo)

# Initialise MQTT
client.create()

# Application specific functions
def getSpeedTest():
    servers = []
    threads = None
    s = Speedtest()
    try:
        s.download(threads=threads)
        s.upload(threads=threads)
        results = s.results.dict()

        # Extract fields and convert to Mbit/s, round ms
        timestamp = results['timestamp']
        downMbit = round((results['download'])/1024/1024)
        upMbit = round((results['upload'])/1024/1024)
        pingMs = round(results['ping'])
        ip = results['client']['ip']
        isp = results['client']['isp']

        data = {"Time": timestamp, "Down": downMbit, "Up": upMbit, "Ping": pingMs, "IP": ip, "isp": isp}

        return data
        
    except Exception as e:
        data = {'data': {'Error': e}}
        client.send_log(f"An error occured during application execution: {e}")

        return 'Error: ' + str(e)


speedTest = getSpeedTest()
if 'error' in speedTest:
    client.send_log(f"Speedtest failed: {speedTest}")
else:
    client.send_data(speedTest)
    client.send_log("Speedtest finished, sending data to data topic")