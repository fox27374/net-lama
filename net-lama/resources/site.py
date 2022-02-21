from flask_restful import Resource, reqparse
from flask_jwt_extended import jwt_required
from models.site import SiteModel

class Site(Resource):
    parser = reqparse.RequestParser()
    parser.add_argument(
        'siteName', 
        type=str,
        required=True,
        help="Cannot be left blank")
    parser.add_argument(
        'orgId',
        type=int,
        required=False,
        help="Optional orgId")

    @jwt_required()
    def get(self, siteId=None):
        # Return a list of all Sites of no siteId is given
        if siteId == None:
            return {'Sites': [site.json() for site in SiteModel.query.all()]}

        site = SiteModel.findById(siteId)
        if site:
            return site.json()
        return {"message": f"Site {siteId} not found"}, 404

    @jwt_required()
    def post(self, siteId=None):
        if siteId:
            return {"message": "siteId not allowed in the request"}, 400

        data = Site.parser.parse_args()

        if SiteModel.findByName(data['siteName']):
            return {"message": f"An Site with the name {data['siteName']} already exists"}, 400
       
        site = SiteModel(**data)

        try:
            site.save()
        except:
            return {"message": "An error occured inserting the item"}, 500 

        return site.json(), 201

    @jwt_required()
    def delete(self, siteId=None):
        if siteId == None:
            return {"message": "siteId has to be send in the request"}, 400

        site = SiteModel.findById(siteId)
        if site:
            site.delete()
            return {"message": f"Site {siteId} deleted"}

        return {"message": f"Site {siteId} not found"}, 404

    @jwt_required()
    def put(self, siteId=None):
        data = Site.parser.parse_args()
        site = SiteModel.findById(siteId)       
        
        # Create a new Site
        if site is None and siteId is None:
            site = SiteModel(**data)
        elif site is None and siteId:
            return {"message": f"Site with siteId {siteId} not found"}, 400
        elif site and siteId is None:
            return {"message": "siteId has to be statet in order to update the Site"}, 400
        else:
            site.siteName = data['siteName']

        site.save()
         
        return site.json()
        
