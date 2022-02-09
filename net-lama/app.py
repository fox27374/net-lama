#!/usr/bin/env python
from sys import path, exit
path.append('../includes/')

from flask import Flask
from flask_restful import Api
#from flask_jwt import JWT, jwt_required
#import apiSchema
#from splib import getCurrentTime, readConfig, writeConfig, writeClientDb, dbHousekeeping
from resources.client import Client
from resources.config import Mqtt
from resources.organization import Organization
from resources.site import Site
from resources.user import User
from db.db import db


app = Flask(__name__)
app.config['SQLALCHEMY_DATABASE_URI'] = 'sqlite:///data.db'
app.config['SQLALCHEMY_TRACK_MODIFICATIONS'] = False
api = Api(app)

@app.before_first_request
def create_tables():
    db.create_all()

#configFile = 'configs/config.json'
#dbFile = 'db/clients.json'
apiBaseUrl = '/api/v1/'
hostIp = '10.140.80.1'
port = 5000
debug = True
#minOutdated = 5

# # Swagger Documentation - TODO
# @app.route(apiBaseUrl + '/docs')
# def spec():
#     return render_template('swaggerui.html')

# # GET requests
# """Get config in whole or by section"""
# @app.route(apiBaseUrl + 'configs/', methods = ['GET'])
# @app.route(apiBaseUrl + 'configs/<section>', methods = ['GET'])
# def getConfigs(section = 'all'):
#     currentConfig = readConfig(configFile)
#     if section == 'all':
#         return jsonify({'configs': list(currentConfig.keys())})
#     else:
#         if section in currentConfig.keys():
#             return jsonify({section: currentConfig[section]})
#         else:
#             abort(404)

# """Get clients"""
# @app.route(apiBaseUrl + 'clients/', methods = ['GET'])
# @app.route(apiBaseUrl + 'clients/<client>', methods = ['GET'])
# def getClients(client = 'all'):
#     currentClients = readConfig(dbFile)
#     if client == 'all':
#         return jsonify(currentClients['clients'])
#     else:
#         if client in currentClients['clients'].keys():
#             return jsonify(currentClients['clients'][client])
#         else:
#             abort(404)

# """Get sites"""
# @app.route(apiBaseUrl + 'sites/', methods = ['GET'])
# @app.route(apiBaseUrl + 'sites/<site>', methods = ['GET'])
# def getSites(site = 'all'):
#     currentSites = readConfig(dbFile)
#     if site == 'all':
#         return jsonify(currentSites['sites'])
#     else:
#         if site in currentSites['sites'].keys():
#             return jsonify(currentSites['sites'][site])
#         else:
#             abort(404)



# # POST requests
# """Update configuration"""
# @app.route(apiBaseUrl + 'configs/update', methods = ['POST'])
# def updateConfig():
#     currentConfig = readConfig(configFile)
#     configSchemaObject = apiSchema.ConfigSchema()
#     postData = request.json
#     error = configSchemaObject.validate(postData)
#     if error:
#         return error
#     else:
#         partialConfig = configSchemaObject.load(postData)
#         section = list(partialConfig.keys())[0]
#         currentConfig[section].update(partialConfig[section])
#         newConfig = currentConfig
#         writeConfig(configFile, currentConfig)
#         return make_response(jsonify({'success': 'true'}), 200)


# """Register client"""
# @app.route(apiBaseUrl + 'clients/register', methods = ['POST'])
# def registerClient():
#     #dbHousekeeping(minOutdated)
#     currentClients = readConfig(dbFile)
#     registerClientSchemaObject = apiSchema.RegisterSchema()
#     postData = request.json
#     error = registerClientSchemaObject.validate(postData)
#     if error:
#         return error
#     else:
#         #newClientId = str(uuid4())[:8]
#         #clientData = {'clientId': newClientId, 'clientType': postData['client']['clientType'], 'lastSeen': getCurrentTime()}
#         clientData = {'clientId': postData['client']['clientId'], 'clientType': postData['client']['clientType'], 'lastSeen': getCurrentTime()}
#         currentClients['clients'].append(clientData)
#         writeClientDb(dbFile, currentClients)
#         return jsonify({'client': clientData})


# """Update client"""
# @app.route(apiBaseUrl + 'clients/update', methods = ['POST'])
# def updateClient():
#     currentClients = readConfig(dbFile)
#     registerClientSchemaObject = apiSchema.RegisterSchema()
#     postData = request.json
#     print(postData)
#     error = registerClientSchemaObject.validate(postData)
#     if error:
#         return error

#     clientId = postData['client']['clientId']
#     inDb = False
#     for client in currentClients['clients']:
#         if client['clientId'] == clientId:
#             inDb = True
#     if inDb == False:
#         abort(404)
#     else:
#         for client in currentClients['clients']:
#             if client['clientId'] == clientId:
#                 client['lastSeen'] = getCurrentTime()
#                 client['clientStatus'] = 'online'
#                 client['appStatus'] = postData['client']['appStatus']
#                 client['capabilities'] = postData['client']['capabilities']
#                 clientData = client
#         writeClientDb(dbFile, currentClients)
#         return jsonify(clientData)

# # WebUI
# @app.route(apiBaseUrl + '/ui')
# def userInterface():
#     return render_template('swaggerui.html')

# # Error handling
# """Error - Not found"""
# @app.errorhandler(404)
# def notFound(error):
#     return make_response(jsonify({'error': 'Not found'}), 404)

#api.add_resource(Client, '/client/<string:clientId>')
#api.add_resource(ClientList, '/clients')
#api.add_resource(Mqtt, f"{apiBaseUrl}/configs")
#api.add_resource(Mqtt, f"{apiBaseUrl}/configs/mqtt/<string:configId>")
#api.add_resource(ClientList, '/configs')
api.add_resource(Organization, f"{apiBaseUrl}/organizations", f"{apiBaseUrl}/organizations/<string:orgId>")
api.add_resource(Site, f"{apiBaseUrl}/sites", f"{apiBaseUrl}/sites/<string:siteId>")
api.add_resource(Client, f"{apiBaseUrl}/clients", f"{apiBaseUrl}/clients/<string:clientId>")
api.add_resource(User, f"{apiBaseUrl}/user", f"{apiBaseUrl}/user/<string:userName>")

if __name__ == '__main__':
    db.init_app(app)
    app.run(host=hostIp, port = port, debug = debug)
