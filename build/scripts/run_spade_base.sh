#!/bin/bash --
export GOMAXPROCS="3" # we need 3 or more for spade to run well..
CORE_COUNT=`grep -c ^processor /proc/cpuinfo`
if [[ $CORE_COUNT -gt $GOMAXPROCS ]];
then
    export GOMAXPROCS=$CORE_COUNT
fi

export GEO_IP_DB="${SPADE_DIR}/config/GeoIPCity.dat"
export ASN_IP_DB="${SPADE_DIR}/config/GeoLiteASNum.dat"
SPADE_DATA_DIR="${SPADE_DIR}/data"
SPADE_LOG_DIR="${SPADE_DIR}/log"

export HOST="$(curl 169.254.169.254/latest/meta-data/hostname)"

STATSD_PREFIX="${OWNER}.${CLOUD_APP}.${CLOUD_DEV_PHASE:-${CLOUD_ENVIRONMENT}}.${EC2_REGION}.${CLOUD_AUTO_SCALE_GROUP##*-}"
mkdir -p ${SPADE_DATA_DIR}/spade_logging ${SPADE_DATA_DIR}/events ${SPADE_DATA_DIR}/upload

export CONFIG_PREFIX="s3://$S3_CONFIG_BUCKET/$VPC_SUBNET_TAG/$CLOUD_APP/$CLOUD_ENVIRONMENT"
aws s3 cp --region us-west-2 "$CONFIG_PREFIX/conf.sh" "$SPADE_DIR/config/conf.sh"
aws s3 cp --region us-west-2 "$CONFIG_PREFIX/conf.json" "$SPADE_DIR/config/conf.json"
source "$SPADE_DIR/config/conf.sh"
export AWS_REGION=us-west-2
export AWS_DEFAULT_REGION=$AWS_REGION # aws-cli uses AWS_DEFAULT_REGION, aws-sdk-go uses AWS_REGION

aws s3 cp "$CONFIG_PREFIX/GeoIPCity.dat" "${GEO_IP_DB}"
aws s3 cp "$CONFIG_PREFIX/GeoLiteASNum.dat" "${ASN_IP_DB}"

# Optional config variables (often set in the s3 conf)
# export MAX_LOG_BYTES=100000000 # 100 MB
# export MAX_LOG_AGE_SECS=3600 # 1 hour
# export MAX_UNTRACKED_LOG_BYTES=10000000 # 10 MB
# export MAX_UNTRACKED_LOG_AGE_SECS=600 # 10 minutes