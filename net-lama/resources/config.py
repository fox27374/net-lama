from flask_restful import Resource, reqparse
from flask_jwt_extended import jwt_required
from models.config import MqttModel, HecForwarderModel, NetworkTestModel
from schemas.config import MqttSchema, HecForwarderSchema, NetworkTestSchema
from webargs.flaskparser import use_args
from json import dumps, dump

class Config(Resource):
    hecForwarderParser = reqparse.RequestParser()
    hecForwarderParser.add_argument('hecServer', type=str, required=True, help="Cannot be left blank")
    hecForwarderParser.add_argument('hecPort', type=int, required=True, help="Cannot be left blank")
    hecForwarderParser.add_argument('hecUrl', type=str, required=True, help="Cannot be left blank")
    hecForwarderParser.add_argument('hecToken', type=str, required=True, help="Cannot be left blank")

    networkTestParser = reqparse.RequestParser()
    networkTestParser.add_argument('speedTestInterval', type=int, required=True, help="Cannot be left blank")
    networkTestParser.add_argument('pingDestination', type=str, required=True, help="Cannot be left blank")
    networkTestParser.add_argument('dnsQuery', type=str, required=True, help="Cannot be left blank")
    networkTestParser.add_argument('dnsServer', type=str, required=True, help="Cannot be left blank")

    @jwt_required()
    def get(self, configType=None, siteId=None):
        # if no configType and no siteId is supplied, return all configs
        if not configType and not siteId:
            return {"configs": {
                        'mqtt': [config.json() for config in MqttModel.query.all()],
                        'hecForwarder': [config.json() for config in HecForwarderModel.query.all()],
                        'networkTest': [config.json() for config in NetworkTestModel.query.all()]
                        }   
                    }, 200

        if configType and not siteId:
            if configType == 'mqtt':
                return {"mqtt": [config.json() for config in MqttModel.query.all()]}, 200
            elif configType == 'hecForwarder':
                return {"hecForwarder": [config.json() for config in HecForwarderModel.query.all()]}, 200
            elif configType == 'networkTest':
                return {"networkTest": [config.json() for config in NetworkTestModel.query.all()]}, 200
            else:
                return {"message": f"configType {configType} not found"}, 404

        if configType and siteId:
            if configType == 'mqtt':
                config = MqttModel.findBySiteId(siteId)
            elif configType == 'hecForwarder':
                config = HecForwarderModel.findBySiteId(siteId)
            elif configType == 'networkTest':
                config = NetworkTestModel.findBySiteId(siteId)
            else:
                return {"message": f"siteId {siteId} not found"}, 404

            mqttSchema = MqttSchema()
            return mqttSchema.dump(config.json())

    @jwt_required()
    @use_args(MqttSchema())
    def post(self, args, configType=None, siteId=None):
        if not configType and not siteId:
            return {"message": f"configType and siteId required"}, 400
        if configType and not siteId:
            return {"message": f"siteId required"}, 400

        if configType and siteId:
            if configType == 'mqtt':
                config = MqttModel(**args, siteId=siteId)

                try:
                    config.save()
                except:
                    return {"message": "an error occured inserting the config"}, 500 

                return config.json(), 201

            if configType == 'hecForwarder':
                
                data = self.hecForwarderParser.parse_args()
                config = HecForwarderModel(**data, siteId=siteId)

                try:
                    config.save()
                except:
                    return {"message": "an error occured inserting the config"}, 500 

                return config.json(), 201

            if configType == 'networkTest':
                
                data = self.networkTestParser.parse_args()
                config = NetworkTestModel(**data, siteId=siteId)

                try:
                    config.save()
                except:
                    return {"message": "an error occured inserting the config"}, 500 

                return config.json(), 201

            else:
                return {"message": f"configType {configType} not found"}, 404

    def delete(self, configType=None, siteId=None):
        if not configType and not siteId:
            return {"message": f"configType and siteId required"}, 400
        if configType and not siteId:
            return {"message": f"siteId required"}, 400

        if configType and siteId:
            if configType == 'mqtt':
                config = MqttModel.findBySiteId(siteId)
            elif configType == 'hecForwarder':
                config = HecForwarderModel.findBySiteId(siteId)
            elif configType == 'networkTest':
                config = NetworkTestModel.findBySiteId(siteId)
            else:
                return {"message": f"siteId {siteId} not found"}, 404

            config.delete()
            return {"message": "Config deleted"}

    @use_args(MqttSchema())
    def put(self, args, configType=None, siteId=None):
        if not configType and not siteId:
            return {"message": f"configType and siteId required"}, 400
        if configType and not siteId:
            return {"message": f"siteId required"}, 400

        if configType and siteId:
            if configType == 'mqtt':
                config = MqttModel.findBySiteId(siteId=siteId)

                if config is None:
                    config = MqttModel(**data, siteId=siteId)

                else:
                    config.mqttServer = args['mqttServer']
                    config.mqttPort = args['mqttPort']
                    config.commandTopic = args['commandTopic']
                    config.dataTopic = args['dataTopic']
                    config.logTopic = args['logTopic']

                try:
                    config.save()
                except:
                    return {"message": "an error occured inserting the config"}, 500 

                return config.json(), 201

            if configType == 'hecForwarder':
                data = self.hecForwarderParser.parse_args()
                config = HecForwarderModel.findBySiteId(siteId=siteId)

                if config is None:
                    config = HecForwarderModel(**data, siteId=siteId)

                else:
                    config.hecServer = data['hecServer']
                    config.hecPort = data['hecPort']
                    config.hecUrl = data['hecUrl']
                    config.hecToken = data['hecToken']

                try:
                    config.save()
                except:
                    return {"message": "an error occured inserting the config"}, 500 

                return config.json(), 201

            if configType == 'networkTest':
                data = self.networkTestParser.parse_args()
                config = NetworkTestModel.findBySiteId(siteId=siteId)

                if config is None:
                    config = NetworkTestModel(**data, siteId=siteId)

                else:
                    config.speedTestInterval = data['speedTestInterval']
                    config.pingDestination = data['pingDestination']
                    config.dnsQuery = data['dnsQuery']
                    config.dnsServer = data['dnsServer']

                try:
                    config.save()
                except:
                    return {"message": "an error occured inserting the config"}, 500 

                return config.json(), 201


class ConfigList(Resource):
    @jwt_required()
    def get(self):
        configs = {}
        configs['mqtt'] = [config.json() for config in MqttModel.query.all()]
        configs['hecForwarder'] = [config.json() for config in HecForwarderModel.query.all()]
        configs['networkTest'] = [config.json() for config in NetworkTestModel.query.all()]

        return configs