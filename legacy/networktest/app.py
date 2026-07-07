#!/usr/bin/env python

from flask import Flask
from flask_restful import Api
#from flask_jwt_extended import JWTManager
#from flask_marshmallow import Marshmallow
from modules.splib import checkApiEndpoint, getClientInfo, registerClient, writeClientInfo
from api.resources.status import Status
from api.resources.start import Start
from api.resources.stop import Stop
from api.resources.update import Update

apiBaseUrl = '/api/v1/'
hostIp = '0.0.0.0'
port = 5001
debug = True

clientInfo = getClientInfo()
app = Flask(__name__)
app.config['PROPAGATE_EXCEPTIONS'] = True
api = Api(app)
  
#jwt = JWTManager(app)

api.add_resource(Status,
        f"{apiBaseUrl}/status"
    )
api.add_resource(Start,
        f"{apiBaseUrl}/start"
    )
api.add_resource(Stop,
        f"{apiBaseUrl}/stop"
    )
api.add_resource(Update,
        f"{apiBaseUrl}/update"
    )

if __name__ == '__main__':
    checkApiEndpoint()
    test = registerClient(clientInfo['clientId'], clientInfo['clientType'])
    clientInfo['siteId'] = test['data']['siteId']
    writeClientInfo(clientInfo)
    app.run(host=hostIp, port=port, debug=debug)
