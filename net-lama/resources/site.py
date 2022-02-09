from flask_restful import Resource, reqparse
from flask_jwt import jwt_required
from models.site import SiteModel

class Site(Resource):
    parser = reqparse.RequestParser()
    parser.add_argument(
        'siteName', 
        type=str,
        required=True,
        help="Cannot be left blank")

    #@jwt_required()
    def get(self, siteId=None):
        # Return a list of all Sites of no siteId is given
        if siteId == None:
            return {'Sites': [org.json() for org in SiteModel.query.all()]}

        org = SiteModel.findById(siteId)
        if org:
            return org.json()
        return {"message": f"Site {siteId} not found"}, 404

    def post(self, siteId=None):
        if siteId:
            return {"message": "siteId not allowed in the request"}, 400

        data = Site.parser.parse_args()

        if SiteModel.findByName(data['siteName']):
            return {"message": f"An Site with the name {data['siteName']} already exists"}, 400
       
        org = SiteModel(**data)

        try:
            org.save()
        except:
            return {"message": "An error occured inserting the item"}, 500 

        return org.json(), 201

    def delete(self, siteId=None):
        if siteId == None:
            return {"message": "siteId has to be send in the request"}, 400

        org = SiteModel.findById(siteId)
        if org:
            org.delete()
            return {"message": f"Site {siteId} deleted"}

        return {"message": f"Site {siteId} not found"}, 404

    def put(self, siteId=None):
        data = Site.parser.parse_args()
        org = SiteModel.findById(siteId)       
        
        # Create a new Site
        if org is None and siteId is None:
            org = SiteModel(**data)
        elif org is None and siteId:
            return {"message": f"Site with siteId {siteId} not found"}, 400
        elif org and siteId is None:
            return {"message": "siteId has to be statet in order to update the Site"}, 400
        else:
            org.siteName = data['siteName']

        org.save()
         
        return org.json()
        
