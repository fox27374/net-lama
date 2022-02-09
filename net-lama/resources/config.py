from flask_restful import Resource, reqparse
#from flask_jwt import jwt_required
from models.config import MqttModel

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

    #@jwt_required()
    def get(self, configId):
        config = MqttModel.find(configId)
        if config:
            return config.json()
        return {"message": "Config not found"}, 404

    def post(self, configId):
        if MqttModel.find(configId):
            return {"message": "a config with this name already exists"}, 400
       
        data = Mqtt.parser.parse_args()
        print(data)
        config = MqttModel(**data)

        try:
            config.save()
        except:
            return {"message": "an error occured inserting the config"}, 500 

        return config.json(), 201

    def delete(self, configId):
        config = MqttModel.find(configId)
        if config:
            config.delete()

        return {"message": "Config deleted"}

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


class ConfigList(Resource):
    def get(self):
        return {'configs': [config.json() for config in MqttModel.query.all()]}
