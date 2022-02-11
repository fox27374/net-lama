from db.db import db

class UserModel(db.Model):
    __tablename__ = 'user'
    id = db.Column(db.Integer, primary_key=True)
    username = db.Column(db.String(80))
    password = db.Column(db.String(80))

    def __init__(self, username, password):
        self.username = username
        self.password = password

    def save(self):
        db.session.add(self)
        db.session.commit()

    def json(self):
        return {
            "username": self.username,
            "password": self.password,
            }

    @classmethod
    def findByName(cls, username):
        return cls.query.filter_by(username=username).first()

    @classmethod
    def findById(cls, userId):
        return cls.query.filter_by(id=userId).first()

    def delete(self):
        db.session.delete(self)
        db.session.commit()