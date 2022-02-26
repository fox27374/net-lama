from flask_restful import Resource
from flask_jwt_extended import jwt_required
from models.config import MqttModel, HecForwarderModel, NetworkTestModel
from schemas.config import MqttSchema, HecForwarderSchema, NetworkTestSchema
from webargs.flaskparser import use_args

class Mqtt(Resource):
    @jwt_required()
    def get(self, siteId=None):
        if not siteId:
            return {"mqtt": [mqtt.json() for mqtt in MqttModel.query.all()]}, 200

        mqtt = MqttModel.findBySiteId(siteId)

        if mqtt:
            return mqtt.json()

        return {"message": f"siteId {siteId} not found"}, 404

    @jwt_required()
    @use_args(MqttSchema())
    def post(self, args, siteId=None):
        if not siteId:
            return {"message": f"siteId required"}, 400

        else:
            mqtt = MqttModel(**args, siteId=siteId)
            try:
                mqtt.save()
            except:
                return {"message": "an error occured inserting the config"}, 500 

            return mqtt.json(), 201

    @jwt_required()
    def delete(self, siteId=None):
        if not siteId:
            return {"message": f"siteId required"}, 400

        else:
            mqtt = MqttModel.findBySiteId(siteId)
            if mqtt:
                mqtt.delete()
                return {"message": f"config deleted successfully"}, 200
            else:
                return {"message": f"siteId {siteId} not found"}, 404

    @jwt_required()
    @use_args(MqttSchema())
    def put(self, args, siteId=None):
        if not siteId:
            return {"message": f"siteId required"}, 400

        else:
            mqtt = MqttModel.findBySiteId(siteId=siteId)
            if not mqtt:
                mqtt = MqttModel(**args, siteId=siteId)

            else:
                mqtt.mqttServer = args['mqttServer']
                mqtt.mqttPort = args['mqttPort']
                mqtt.commandTopic = args['commandTopic']
                mqtt.dataTopic = args['dataTopic']
                mqtt.logTopic = args['logTopic']
                mqtt.siteId = siteId

            try:
                mqtt.save()
                return mqtt.json(), 201
            except:
                return {"message": "an error occured inserting the config"}, 500 


class HecForwarder(Resource):
    @jwt_required()
    def get(self, siteId=None):
        if not siteId:
            return {"hecForwarder": [hecForwarder.json() for hecForwarder in HecForwarderModel.query.all()]}, 200

        hecForwarder = HecForwarderModel.findBySiteId(siteId)

        if hecForwarder:
            return hecForwarder.json()

        return {"message": f"siteId {siteId} not found"}, 404

    @jwt_required()
    @use_args(HecForwarderSchema())
    def post(self, args, siteId=None):
        if not siteId:
            return {"message": f"siteId required"}, 400

        else:
            hecForwarder = HecForwarderModel(**args, siteId=siteId)
            try:
                hecForwarder.save()
            except:
                return {"message": "an error occured inserting the config"}, 500 

            return hecForwarder.json(), 201

    @jwt_required()
    def delete(self, siteId=None):
        if not siteId:
            return {"message": f"siteId required"}, 400

        else:
            hecForwarder = HecForwarderModel.findBySiteId(siteId)
            if hecForwarder:
                hecForwarder.delete()
                return {"message": f"config deleted successfully"}, 200
            else:
                return {"message": f"siteId {siteId} not found"}, 404

    @jwt_required()
    @use_args(HecForwarderSchema())
    def put(self, args, siteId=None):
        if not siteId:
            return {"message": f"siteId required"}, 400

        else:
            hecForwarder = HecForwarderModel.findBySiteId(siteId=siteId)
            if not hecForwarder:
                hecForwarder = HecForwarderModel(**args, siteId=siteId)

            else:
                hecForwarder.hecServer = args['hecServer']
                hecForwarder.hecPort = args['hecPort']
                hecForwarder.hecUrl = args['hecUrl']
                hecForwarder.hecToken = args['hecToken']
                hecForwarder.siteId = siteId

            try:
                hecForwarder.save()
                return hecForwarder.json(), 201
            except:
                return {"message": "an error occured inserting the config"}, 500 


class NetworkTest(Resource):
    @jwt_required()
    def get(self, siteId=None):
        if not siteId:
            return {"networkTest": [networkTest.json() for networkTest in NetworkTestModel.query.all()]}, 200

        networkTest = NetworkTestModel.findBySiteId(siteId)

        if networkTest:
            return networkTest.json()

        return {"message": f"siteId {siteId} not found"}, 404

    @jwt_required()
    @use_args(NetworkTestSchema())
    def post(self, args, siteId=None):
        if not siteId:
            return {"message": f"siteId required"}, 400

        else:
            networkTest = NetworkTestModel(**args, siteId=siteId)
            try:
                networkTest.save()
            except:
                return {"message": "an error occured inserting the config"}, 500 

            return networkTest.json(), 201

    @jwt_required()
    def delete(self, siteId=None):
        if not siteId:
            return {"message": f"siteId required"}, 400

        else:
            networkTest = NetworkTestModel.findBySiteId(siteId)
            if networkTest:
                networkTest.delete()
                return {"message": f"config deleted successfully"}, 200
            else:
                return {"message": f"siteId {siteId} not found"}, 404

    @jwt_required()
    @use_args(NetworkTestSchema())
    def put(self, args, siteId=None):
        if not siteId:
            return {"message": f"siteId required"}, 400

        else:
            networkTest = NetworkTestModel.findBySiteId(siteId=siteId)
            if not networkTest:
                networkTest = NetworkTestModel(**args, siteId=siteId)

            else:
                networkTest.speedTestInterval = args['speedTestInterval']
                networkTest.pingDestination = args['pingDestination']
                networkTest.dnsQuery = args['dnsQuery']
                networkTest.dnsServer = args['dnsServer']
                networkTest.siteId = siteId

            try:
                networkTest.save()
                return networkTest.json(), 201
            except:
                return {"message": "an error occured inserting the config"}, 500 