#!/usr/bin/env python

from sys import path, exit
path.append('../includes/')

from splib import Client, checkApiEndpoint, getClientInfo, getConfig, registerClient, updateClient
from time import sleep
from speedtest import Speedtest
from subprocess import Popen, PIPE

from sys import argv
from getopt import getopt
from subprocess import Popen
from os import getcwd

argv = argv[1:]

try:
    opts, args = getopt(argv, "h:s:")
except:
    print("Error")

for opt, arg in opts:
    if opt in ['-h']:
        host = arg

# Read local client config
localConfig = getClientInfo()

clientInfo = {}
clientInfo['clientId'] = localConfig['clientId']
clientInfo['clientType'] = localConfig['clientType']
clientInfo['commands'] = localConfig['commands']
clientInfo['capabilities'] = localConfig['capabilities']

# Wait until the API endpoint is available
#checkApiEndpoint()

# Get MQTT config
mqttConfig = getConfig('configs/MQTT')
clientInfo['mqttServer'] = mqttConfig['MQTT']['mqttServer']
clientInfo['mqttPort'] = mqttConfig['MQTT']['mqttPort']
clientInfo['commandTopic'] = mqttConfig['MQTT']['commandTopic']
clientInfo['dataTopic'] = mqttConfig['MQTT']['dataTopic']
clientInfo['logTopic'] = mqttConfig['MQTT']['logTopic']

# Get Application specific config
networkTestConfig = getConfig('configs/NetworkTest')
speedTestInterval = networkTestConfig['NetworkTest']['speedTestInterval']
pingDestination = networkTestConfig['NetworkTest']['pingDestination']
dnsQuery = networkTestConfig['NetworkTest']['dnsQuery']
dnsServer = networkTestConfig['NetworkTest']['dnsServer']

# Create client object
client = Client(**clientInfo)

# Initialise MQTT
client.create()
client.connect()

# Initialise application
client.cmdQueue = ['start']

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
        client.log(f"An error occured during application execution: {e}")

        return 'Error: ' + e



# Register client and get ID used for further communication
# Exit if registration fails
#register = registerClient(client.clientType, client.clientId)
#if register['status'] == 'ok': clientId = register['data']['client']['clientId']
#else:
#    print(f"An error occured: {register['data']}")
#    exit()

# Update client information at api endpoint
#if client.cmdQueue[-1] == 'start': appStatus = 'running'
#elif client.cmdQueue[-1] == 'stop': appStatus = 'stopped'
#else: appStatus = 'undefined'
#updateClient(client.clientId, client.clientType, appStatus, client.capabilities)


# Main task, controlled by the cmdQueue switch
speedTest = getSpeedTest()
if 'error' in speedTest:
    client.log(f"Speedtest failed: {speedTest}")
else:
    client.data(speedTest)
    client.log("Speedtest finished, sending data to data topic")