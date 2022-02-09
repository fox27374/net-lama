from flask_restful import Resource, reqparse
from flask_jwt import jwt_required
from models.organization import OrganizationModel

class Organization(Resource):
    parser = reqparse.RequestParser()
    parser.add_argument(
        'orgName', 
        type=str,
        required=True,
        help="Cannot be left blank")

    #@jwt_required()
    def get(self, orgId=None):
        # Return a list of all organizations of no orgId is given
        if orgId == None:
            return {'organizations': [org.json() for org in OrganizationModel.query.all()]}

        org = OrganizationModel.findById(orgId)
        if org:
            return org.json()
        return {"message": f"Organization {orgId} not found"}, 404

    def post(self, orgId=None):
        if orgId:
            return {"message": "orgId not allowed in the request"}, 400

        data = Organization.parser.parse_args()

        if OrganizationModel.findByName(data['orgName']):
            return {"message": f"An organization with the name {data['orgName']} already exists"}, 400
       
        org = OrganizationModel(**data)

        try:
            org.save()
        except:
            return {"message": "An error occured inserting the item"}, 500 

        return org.json(), 201

    def delete(self, orgId=None):
        if orgId == None:
            return {"message": "orgId has to be send in the request"}, 400

        org = OrganizationModel.findById(orgId)
        if org:
            org.delete()
            return {"message": f"Organization {orgId} deleted"}
        
        return {"message": f"Organization {orgId} not found"}, 404

    def put(self, orgId=None):
        data = Organization.parser.parse_args()
        org = OrganizationModel.findById(orgId)       
        
        # Create a new organization
        if org is None and orgId is None:
            org = OrganizationModel(**data)
        elif org is None and orgId:
            return {"message": f"Organization with orgId {orgId} not found"}, 404
        elif org and orgId is None:
            return {"message": "orgId has to be statet in order to update the organization"}, 400
        else:
            org.orgName = data['orgName']

        org.save()
         
        return org.json()

