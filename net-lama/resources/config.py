from flask_restful import Resource, reqparse
from flask_jwt import jwt_required
from models.config import MqttModel, HecForwarderModel, NetworkTestModel
from json import dumps, dump

class Mqtt(Resource):
    parser = reqparse.RequestParser()
    parser.add_argument(
        'mqttServer', 
        type=str,
        required=True,
        help="Cannot be left blank")
    parser.add_argument(
        'mqttPort', 
        type=int,
        required=True,
        help="Cannot be left blank")
    parser.add_argument(
        'commandTopic', 
        type=str,
        required=True,
        help="Cannot be left blank")
    parser.add_argument(
        'dataTopic', 
        type=str,
        required=True,
        help="Cannot be left blank")
    parser.add_argument(
        'logTopic', 
        type=str,
        required=True,
        help="Cannot be left blank")

    @jwt_required()
    def get(self, configId=None):
        if configId == None:
            return {'mqtt': [config.json() for config in MqttModel.query.all()]}

        config = MqttModel.find(configId)
        if config:
            return config.json()
        return {"message": "Config not found"}, 404

    @jwt_required()
    def post(self, configId=None):
        if configId:
            return {"message": "configId not allowed in the request"}, 400
      
        data = Mqtt.parser.parse_args()
        config = MqttModel(**data)

        try:
            config.save()
        except:
            return {"message": "an error occured inserting the config"}, 500 

        return config.json(), 201

    @jwt_required()
    def delete(self, configId):
        config = MqttModel.find(configId)
        if config:
            config.delete()

        return {"message": "Config deleted"}

    @jwt_required()
    def put(self, configId):
        data = Mqtt.parser.parse_args()
        config = MqttModel.find(configId)
        

        if config is None:
            config = MqttModel(**data)
        else:
            mqttServer = data['mqttServer']
            mqttPort = data['mqttPort']
            commandTopic = data['commandTopic']
            dataTopic = data['dataTopic']
            logTopic = data['logTopic']

        config.save()
         
        return config.json()


class HecForwarder(Resource):
    parser = reqparse.RequestParser()
    parser.add_argument(
        'hecServer', 
        type=str,
        required=True,
        help="Cannot be left blank")
    parser.add_argument(
        'hecPort', 
        type=int,
        required=True,
        help="Cannot be left blank")
    parser.add_argument(
        'hecUrl', 
        type=str,
        required=True,
        help="Cannot be left blank")
    parser.add_argument(
        'hecToken', 
        type=str,
        required=True,
        help="Cannot be left blank")

    @jwt_required()
    def get(self, configId=None):
        if configId == None:
            return {'mqtt': [config.json() for config in HecForwarderModel.query.all()]}

        config = HecForwarderModel.find(configId)
        if config:
            return config.json()
        return {"message": "Config not found"}, 404

    @jwt_required()
    def post(self, configId=None):
        if configId:
            return {"message": "configId not allowed in the request"}, 400
      
        data = HecForwarder.parser.parse_args()
        config = HecForwarderModel(**data)

        try:
            config.save()
        except:
            return {"message": "an error occured inserting the config"}, 500 

        return config.json(), 201

    @jwt_required()
    def delete(self, configId):
        config = HecForwarderModel.find(configId)
        if config:
            config.delete()

        return {"message": "Config deleted"}

    @jwt_required()
    def put(self, configId):
        data = Mqtt.parser.parse_args()
        config = HecForwarderModel.find(configId)
        

        if config is None:
            config = HecForwarderModel(**data)
        else:
            mqttServer = data['mqttServer']
            mqttPort = data['mqttPort']
            commandTopic = data['commandTopic']
            dataTopic = data['dataTopic']
            logTopic = data['logTopic']

        config.save()
         
        return config.json()


class NetworkTest(Resource):
    parser = reqparse.RequestParser()
    parser.add_argument(
        'speedTestInterval', 
        type=int,
        required=True,
        help="Cannot be left blank")
    parser.add_argument(
        'pingDestination', 
        type=str,
        required=True,
        help="Cannot be left blank")
    parser.add_argument(
        'dnsQuery', 
        type=str,
        required=True,
        help="Cannot be left blank")
    parser.add_argument(
        'dnsServer', 
        type=str,
        required=True,
        help="Cannot be left blank")

    @jwt_required()
    def get(self, configId=None):
        if configId == None:
            return {'mqtt': [config.json() for config in NetworkTestModel.query.all()]}

        config = NetworkTestModel.find(configId)
        if config:
            return config.json()
        return {"message": "Config not found"}, 404

    @jwt_required()
    def post(self, configId=None):
        if configId:
            return {"message": "configId not allowed in the request"}, 400
      
        data = NetworkTest.parser.parse_args()
        data['pingDestination'] = dumps(data['pingDestination'])
        data['dnsQuery'] = dumps(data['dnsQuery'])
        data['dnsServer'] = dumps(data['dnsServer'])
        config = NetworkTestModel(**data)

        try:
            config.save()
        except Exception as e:
            return {"message": f"an error occured inserting the config: {e}"}, 500 

        return config.json(), 201

    @jwt_required()
    def delete(self, configId):
        config = NetworkTestModel.find(configId)
        if config:
            config.delete()

        return {"message": "Config deleted"}

    @jwt_required()
    def put(self, configId):
        data = Mqtt.parser.parse_args()
        config = NetworkTestModel.find(configId)
        

        if config is None:
            config = NetworkTestModel(**data)
        else:
            mqttServer = data['mqttServer']
            mqttPort = data['mqttPort']
            commandTopic = data['commandTopic']
            dataTopic = data['dataTopic']
            logTopic = data['logTopic']

        config.save()
         
        return config.json()


class ConfigList(Resource):
    @jwt_required()
    def get(self):
        configs = {}
        configs['mqtt'] = [config.json() for config in MqttModel.query.all()]
        configs['hecForwarder'] = [config.json() for config in HecForwarderModel.query.all()]
        configs['networkTest'] = [config.json() for config in NetworkTestModel.query.all()]

        return configs