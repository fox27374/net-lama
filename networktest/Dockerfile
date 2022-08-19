# ./Dockerfile

FROM python:3.8-alpine

# Labeling
LABEL "org.lnxnet.project"="net-lama"
LABEL "org.lnxnet.application"="networktest"

# Set variables
ENV USER=net-lama
ENV GROUP=net-lama
ENV HOME /home/$USER

# Install software
RUN apk add --no-cache bind-tools iputils

# Create user and group
RUN addgroup $GROUP
RUN adduser -G $GROUP -h $HOME -D $USER

# Use new user and directory
USER $USER
WORKDIR $HOME

# Copy source files
COPY --chown=$USER:$GROUP clientInfo.json .
COPY --chown=$USER:$GROUP requirements.txt .

#COPY --chown=$USER:$GROUP globalVars.py .
COPY --chown=$USER:$GROUP networkTest.py .
#COPY --chown=$USER:$GROUP splib.py .

# Set environment to the net-lama directory
ENV VIRTUAL_ENV=$HOME/venv
RUN python3 -m venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# Install dependencies:
RUN pip install --no-cache-dir -r requirements.txt

# Run the application:
CMD ["python", "networkTest.py"]
