from db.db import db

class SiteModel(db.Model):
    __tablename__ = 'sites'
    siteId = db.Column(db.Integer, primary_key=True)
    orgId = db.Column(db.Integer)
    siteName = db.Column(db.String(80))

    def __init__(self, siteName, orgId=None):
        self.siteName = siteName
        self.orgId = orgId if orgId else 1

    def json(self):
        return {
            "siteId": self.siteId,
            "orgId": self.orgId,
            "siteName": self.siteName
            }

    @classmethod
    def findBySiteId(cls, siteId):
        return cls.query.filter_by(siteId=siteId).first()

    @classmethod
    def findByOrgId(cls, orgId):
        return cls.query.filter_by(orgId=orgId)

    @classmethod
    def findByName(cls, siteName):
        return cls.query.filter_by(siteName=siteName).first()

    def save(self):
        db.session.add(self)
        db.session.commit()

    def delete(self):
        db.session.delete(self)
        db.session.commit()
    