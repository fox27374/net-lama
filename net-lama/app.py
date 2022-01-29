#!/usr/bin/env python

from flask import Flask, jsonify, make_response, request, abort, render_template
import apiSchema
import json
from datetime import datetime, timedelta
from uuid import uuid4

app = Flask(__name__)

configFile = 'configs/config.json'
dbFile = 'db/clients.json'
apiBaseUrl = '/api/v1/'
hostIp = '0.0.0.0'
#hostIp = '10.140.80.1'
port = 5000
debug = False
minOutdated = 5

def getCurrentTime():
    now = datetime.now()
    currentTime = now.strftime('%Y-%m-%d %H:%M:%S')
    return currentTime

def readConfig(configFile):
    with open(configFile, 'r') as cf:
        configDict = json.load(cf)
    return configDict

def writeConfig(configFile, configData):
    cf = open(configFile, 'w')
    cf.write(json.dumps(configData, indent=4))
    cf.close()

def writeClientDb(dbFile, clientData):
    cf = open(dbFile, 'w')
    cf.write(json.dumps(clientData, indent=4))
    cf.close()

def dbHousekeeping(minOutdated):
    now = datetime.now()
    compareTime = now - timedelta(minutes=minOutdated)
    print(compareTime)
    currentClients = readConfig(dbFile)
    newClients = []
    for client in currentClients['clients']:
        print(client['lastSeen'])
        print(client['clientId'])
        lastSeen = datetime.strptime(client['lastSeen'], '%Y-%m-%d %H:%M:%S')
        if lastSeen > compareTime:
            newClients.append(client)
    currentClients['clients'] = newClients
    writeClientDb(dbFile, currentClients)

# Swagger Documentation - TODO
@app.route(apiBaseUrl + '/docs')
def spec():
    return render_template('swaggerui.html')

# GET requests
"""Get config in whole or by section"""
@app.route(apiBaseUrl + 'configs/', methods = ['GET'])
@app.route(apiBaseUrl + 'configs/<section>', methods = ['GET'])
def getConfigs(section = 'all'):
    currentConfig = readConfig(configFile)
    if section == 'all':
        return jsonify({'configs': list(currentConfig.keys())})
    else:
        if section in currentConfig.keys():
            return jsonify({section: currentConfig[section]})
        else:
            abort(404)

"""Get clients"""
@app.route(apiBaseUrl + 'clients/', methods = ['GET'])
@app.route(apiBaseUrl + 'clients/<client>', methods = ['GET'])
def getClients(client = 'all'):
    currentClients = readConfig(dbFile)
    if client == 'all':
        return jsonify(currentClients['clients'])
    else:
        if client in currentClients['clients'].keys():
            return jsonify(currentClients['clients'][client])
        else:
            abort(404)

"""Get sites"""
@app.route(apiBaseUrl + 'sites/', methods = ['GET'])
@app.route(apiBaseUrl + 'sites/<site>', methods = ['GET'])
def getSites(site = 'all'):
    currentSites = readConfig(dbFile)
    if site == 'all':
        return jsonify(currentSites['sites'])
    else:
        if site in currentSites['sites'].keys():
            return jsonify(currentSites['sites'][site])
        else:
            abort(404)



# POST requests
"""Update configuration"""
@app.route(apiBaseUrl + 'configs/update', methods = ['POST'])
def updateConfig():
    currentConfig = readConfig(configFile)
    configSchemaObject = apiSchema.ConfigSchema()
    postData = request.json
    error = configSchemaObject.validate(postData)
    if error:
        return error
    else:
        partialConfig = configSchemaObject.load(postData)
        section = list(partialConfig.keys())[0]
        currentConfig[section].update(partialConfig[section])
        newConfig = currentConfig
        writeConfig(configFile, currentConfig)
        return make_response(jsonify({'success': 'true'}), 200)


"""Register client"""
@app.route(apiBaseUrl + 'clients/register', methods = ['POST'])
def registerClient():
    dbHousekeeping(minOutdated)
    currentClients = readConfig(dbFile)
    registerClientSchemaObject = apiSchema.RegisterSchema()
    postData = request.json
    error = registerClientSchemaObject.validate(postData)
    if error:
        return error
    else:
        #newClientId = str(uuid4())[:8]
        #clientData = {'clientId': newClientId, 'clientType': postData['client']['clientType'], 'lastSeen': getCurrentTime()}
        clientData = {'clientId': postData['client']['clientId'], 'clientType': postData['client']['clientType'], 'lastSeen': getCurrentTime()}
        currentClients['clients'].append(clientData)
        writeClientDb(dbFile, currentClients)
        return jsonify({'client': clientData})


"""Update client"""
@app.route(apiBaseUrl + 'clients/update', methods = ['POST'])
def updateClient():
    currentClients = readConfig(dbFile)
    registerClientSchemaObject = apiSchema.RegisterSchema()
    postData = request.json
    print(postData)
    error = registerClientSchemaObject.validate(postData)
    if error:
        return error

    clientId = postData['client']['clientId']
    inDb = False
    for client in currentClients['clients']:
        if client['clientId'] == clientId:
            inDb = True
    if inDb == False:
        abort(404)
    else:
        for client in currentClients['clients']:
            if client['clientId'] == clientId:
                client['lastSeen'] = getCurrentTime()
                client['clientStatus'] = 'online'
                client['appStatus'] = postData['client']['appStatus']
                client['capabilities'] = postData['client']['capabilities']
                clientData = client
        writeClientDb(dbFile, currentClients)
        return jsonify(clientData)

# WebUI
@app.route(apiBaseUrl + '/ui')
def userInterface():
    return render_template('swaggerui.html')

# Error handling
"""Error - Not found"""
@app.errorhandler(404)
def notFound(error):
    return make_response(jsonify({'error': 'Not found'}), 404)

app.run(host=hostIp, port = port, debug = debug)
