from db.db import db

class UserModel(db.Model):
    __tablename__ = 'user'
    _id = db.Column(db.Integer, primary_key=True)
    userName = db.Column(db.String(80))
    userPass = db.Column(db.String(80))

    def __init__(self, userName, userPass):
        self.userName = userName
        self.userPass = userPass

    def save(self):
        db.session.add(self)
        db.session.commit()

    def json(self):
        return {
            "userName": self.userName,
            "userPass": self.userPass,
            }

    @classmethod
    def findByName(cls, userName):
        print("xxx" + cls.query.filter_by(userName=userName).first())
        return cls.query.filter_by(userName=userName).first()

    @classmethod
    def findById(cls, userId):
        return cls.query.filter_by(_id=userId).first()

    def delete(self):
        db.session.delete(self)
        db.session.commit()