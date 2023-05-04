# Sample intervals
These sample intervals are for the cloud.

## Cargo
### Sample interval
1 minute

### Paths
* cargo.tank.\*.capacity
* cargo.tank.\*.currentLevel
* cargo.tank.\*.density
* cargo.tank.\*.temperature 
* cargo.total.capacity
* cargo.total.currentLevel
* design.displacement
* design.draft.\*

## Vessel properies
### Sample interval
15 minute

### Paths
* communication.callsignVhf
* design.aisShipType
* design.beam
* design.length
* mmsi
* name
* registrations.imo
* registrations.other.eni.registration
* navigation.state

## Vessel position and movement
### Sample interval
5 seconds

### Paths
* environment.heave
* navigation.courseOverGroundTrue
* navigation.headingTrue
* navigation.position
* navigation.rateOfTurn
* navigation.speedOverGround
* steering.rudderAngle

## Environment, meteo
### Sample interval
10 seconds

### Paths
* environment.wind.direction\*
* environment.wind.speed\*
* environment.outside.temperature
* environment.outside.pressure
* environment.inside.engineRoom.temperature

## Environment, depth
### Sample interval
10 seconds

### Paths
* environment.depth.belowTransducer

## GNSS, information
### Sample interval
1 minute

### Paths
* navigation.datetime
* navigation.gnss.methodQuality
* navigation.gnss.satellites
* navigation.gnss.type

### Power
### Sample interval
1 second

### Paths
* propulsion.\*.drive.power
* propulsion.\*.drive.revolutions
* propulsion.\*.drive.torque

### Engine conditions
### Sample interval
5 seconds

### Paths
* _(none yet)_

## Fuel
### Sample interval
5 seconds _(wait for feedback from suppliers)_

### Paths
* propulsion.\*.fuel.rate
* propulsion.\*.fuel.rate.return
* propulsion.\*.fuel.rate.supply
* propulsion.\*.fuel.temperature


## Tanks
### Sample interval
1 minute

### Paths
tanks.\*.\*.currentVolume

## Tanks
### Sample interval
5 seconds _(to be determined)_

### Paths
notifications.ais
notifications.bilge.\*
notifications.electrical.batteries.main
notifications.tanks.\*.\*
