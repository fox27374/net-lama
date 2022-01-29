#!/usr/bin/env python

from sys import path, exit
path.append('../includes/')

from splib import *
from time import sleep
import speedtest
import subprocess
import re

# Read local client config
localConfig = getClientInfo()

clientInfo = {}
clientInfo['clientId'] = localConfig['clientId']
clientInfo['clientType'] = localConfig['clientType']
clientInfo['commands'] = localConfig['commands']
clientInfo['capabilities'] = localConfig['capabilities']

# Wait until the API endpoint is available
checkApiEndpoint()

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
def getPingTime(host):
    """Ping a host and return the average round-trip-time"""
    command = ['ping', '-4', '-n', '-i', '0.2', '-c', '5', host]

    p = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    output, errors = p.communicate()
    output = output.decode("utf-8").splitlines()

    avgTimeMs = 'NA'
    timeMs = []

    for line in output:
        if 'time=' in line:
            ms = re.findall('time=(\d+\.\d+)', line)
            
            if ms:
                timeMs.append(float(ms[0]))
            else:
                ms = 3000
                timeMs.append(float(ms))

            avgTimeMs = round((sum(timeMs)/len(timeMs)), 2)

    data = {"Test": "Ping", "Host": host, "Time": avgTimeMs}

    return data

def getDnsTime(host, server):
    """Do a DNS lookup and retuen the query time"""

    command = ['dig', '-4', '-u', '+timeout=1', '@' + server, host]

    p = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    output, errors = p.communicate()
    output = output.decode("utf-8").splitlines()

    for line in output:
        if 'Query time' in line:
            ms = re.findall('Query\stime:\s(\d+)', line)
            ms = round(float(ms[0])/1024, 2)

        if 'timed out' in line:
            ms = 3000

    data = {"Test": "DNS", "Host": host, "Server": server, "Time": ms}

    return data

def getSpeedTest():
    servers = []
    threads = None
    s = speedtest.Speedtest()
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
register = registerClient(client.clientType, client.clientId)
if register['status'] == 'ok': clientId = register['data']['client']['clientId']
else:
    print(f"An error occured: {register['data']}")
    exit()

# Update client information at api endpoint
if client.cmdQueue[-1] == 'start': appStatus = 'running'
elif client.cmdQueue[-1] == 'stop': appStatus = 'stopped'
else: appStatus = 'undefined'
updateClient(client.clientId, client.clientType, appStatus, client.capabilities)


# Main task, controlled by the cmdQueue switch
while True:
    if client.cmdQueue[-1] == 'start':
        try:
            counter = int(speedTestInterval)

            while counter >= 0:
                for destination in pingDestination:
                    pingTime = getPingTime(destination)
                    client.data(pingTime)
                for server in dnsServer:
                    for query in dnsQuery:
                        dnsTime = getDnsTime(query, server)
                        client.data(dnsTime)

                client.log("Ping and DNS test finished")
                counter = counter - 1
                sleep(1)

            speedTest = getSpeedTest()
            if 'error' in speedTest:
                client.log(f"Speedtest failed: {speedTest}")
            else:
                client.data(speedTest)
                client.log("Speedtest finished, sending data to data topic")
                counter = int(speedTestInterval)

        except Exception as e:
            #data = {'clientId': clientId, 'clientType': clientType, 'data': {'Error': e}}
            client.log(f"An error occured during application execution: {e}")

    else:
        pass
