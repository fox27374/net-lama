from flask_restful import Resource, reqparse
from flask_jwt_extended import jwt_required
from models.mapping import MappingModel

class Mapping(Resource):
    parser = reqparse.RequestParser()
    parser.add_argument(
        'orgName', 
        type=str,
        required=True,
        help="Cannot be left blank")

    @jwt_required()
    def get(self):
        # Return a list of all organizations of no orgId is given
        if orgId == None:
            return {'organizations': [mapping.json() for mapping in MappingModel.query.all()]}

        mapping = MappingModel.findById(orgId)
        if mapping:
            return mapping.json()
        return {"message": f"Organization {orgId} not found"}, 404

    @jwt_required()
    def post(self, orgId=None):
        if orgId:
            return {"message": "orgId not allowed in the request"}, 400

        data = Organization.parser.parse_args()

        if MappingModel.findByName(data['orgName']):
            return {"message": f"An organization with the name {data['orgName']} already exists"}, 400
       
        org = MappingModel(**data)

        try:
            org.save()
        except:
            return {"message": "An error occured inserting the item"}, 500 

        return org.json(), 201

    @jwt_required()
    def delete(self, orgId=None):
        if orgId == None:
            return {"message": "orgId has to be send in the request"}, 400

        org = MappingModel.findById(orgId)
        if org:
            org.delete()
            return {"message": f"Organization {orgId} deleted"}
        
        return {"message": f"Organization {orgId} not found"}, 404

    @jwt_required()
    def put(self, orgId=None):
        data = Organization.parser.parse_args()
        org = MappingModel.findById(orgId)       
        
        # Create a new organization
        if org is None and orgId is None:
            org = MappingModel(**data)
        elif org is None and orgId:
            return {"message": f"Organization with orgId {orgId} not found"}, 404
        elif org and orgId is None:
            return {"message": "orgId has to be statet in order to update the organization"}, 400
        else:
            org.orgName = data['orgName']

        org.save()
         
        return org.json()

