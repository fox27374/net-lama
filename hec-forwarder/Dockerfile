# ./Dockerfile

FROM python:3.8-alpine

# Labeling
LABEL "org.lnxnet.project"="net-lama"
LABEL "org.lnxnet.application"="hec-forwarder"

# Set variables
ENV USER=net-lama
ENV GROUP=net-lama
ENV HOME /home/$USER

# Create user and group
RUN addgroup $GROUP
RUN adduser -G $GROUP -h $HOME -D $USER

# Use new user and directory
USER $USER
WORKDIR $HOME

# Copy source files
COPY . .

# Set environment to the net-lama directory
ENV VIRTUAL_ENV=$HOME/venv
RUN python3 -m venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# Install dependencies:
RUN pip install --no-cache-dir -r requirements.txt

# Run the application:
CMD ["python", "hecForwarder.py"]
