#!/usr/bin/env python
from sys import path
path.append('../includes/')

from flask import Flask
from flask_restful import Api
from flask_jwt_extended import JWTManager
from flask_marshmallow import Marshmallow
#import apiSchema
#from splib import getCurrentTime, readConfig, writeConfig, writeClientDb, dbHousekeeping
from resources.client import Client
from resources.config import Config
from resources.organization import Organization
from resources.site import Site
from resources.user import User, UserLogin, UserHello
from db.db import db
#from datetime import timedelta

apiBaseUrl = '/api/v1/'
hostIp = '10.140.80.1'
port = 5000
debug = True

app = Flask(__name__)
app.config['SQLALCHEMY_DATABASE_URI'] = 'sqlite:///data.db'
app.config['SQLALCHEMY_TRACK_MODIFICATIONS'] = False
app.config['PROPAGATE_EXCEPTIONS'] = True
app.secret_key = 'test'
api = Api(app)
ma = Marshmallow(app)

@app.before_first_request
def create_tables():
    db.create_all()

jwt = JWTManager(app)


api.add_resource(Organization,
        f"{apiBaseUrl}/organizations",
        f"{apiBaseUrl}/organizations/<string:orgId>"
    )
api.add_resource(Site,
        f"{apiBaseUrl}/sites",
        f"{apiBaseUrl}/sites/<string:siteId>"
    )
api.add_resource(Client,
        f"{apiBaseUrl}/clients",
        f"{apiBaseUrl}/clients/<string:clientId>"
    )
api.add_resource(User,
        f"{apiBaseUrl}/user",
        f"{apiBaseUrl}/user/<string:userName>"
    )
api.add_resource(Config,
        f"{apiBaseUrl}/configs",
        f"{apiBaseUrl}/configs/<string:configType>",
        f"{apiBaseUrl}/configs/<string:configType>/<int:siteId>"
    )
api.add_resource(UserLogin,
        f"{apiBaseUrl}/login"
    )
api.add_resource(UserHello,
        f"{apiBaseUrl}/hello"
    )

if __name__ == '__main__':
    db.init_app(app)
    app.run(host=hostIp, port = port, debug = debug)
