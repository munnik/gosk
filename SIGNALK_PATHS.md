# SignalK paths

Extra information for (non standard) SignalK paths used in GOSK

## 1. Identifier

The regular expression part in a path is used to identify the function and location of equipment.

### 1.1 Function

If the function can be determined by the path itself, e.g. `/vessels/<RegExp>/tanks/fuel/<RegExp>` => `/vessels/<RegExp>/tanks/fuel/starboardAft`, the function is not included in the regular expression.

Used functions in GOSK are:

- `mainEngine`
- `generatorEngine`
- `bowThrusterEngine`
- `auxiliaryEngine`

### 1.2 Location

The location is not included in the path when there is only one instance of that equipment, e.g. `/vessels/<RegExp>/propulsion/<RegExp>` => `/vessels/<RegExp>/propulsion/mainEngine`.

If multiple instances of the same equipment exist then a location is added after the function, e.g. `/vessels/<RegExp>/propulsion/mainEnginePortAft` and `/vessels/<RegExp>/propulsion/mainEngineStarboardAft`. If multiple instances of the same equipment exist then a path without location must exist and should contain an aggregate value, e.g. `/vessels/<RegExp>/propulsion/mainEngine`.

#### 1.2.1 Port to starboard

Used locations in GOSK are:

- `port` (cannot be used together with `portInner` and `portOuter`)
- `portOuter`
- `portInner`
- `centerPort` (port of center, can be used with `port` or with `portInner` and `portOuter`)
- `center`
- `centerStarboard` (starboard of center, can be used with `starboard` or with `starboardInner` and `starboardOuter`)
- `starboardInner`
- `starboardOuter`
- `starboard` (cannot be used together with `starboardInner` and `starboardOuter`)

#### 1.2.2 Forward to aft

Used locations in GOSK are:

- `forward` (in front of the cargo area)
- `aft` (behind the cargo area)
- `<number>` (can be added to a position to clarify a position, lower numbers are closer to the front of the ship, higher numbers are closer to the aft of the ship, counting starts at 1)

#### 1.2.3 Combining locations

A complete location is the combination of the above written as camelCase. The combination starts with optional a 'port to starboard' location and then optional a 'forward to aft' location.

Valid examples are:

- `<empty string>` (used when for single instance or aggregate)
- `forward`
- `starboardInner`
- `portAft1`

### 1.3 Combining function and location

A complete identifier is a combination of a function and a location written in camelCase. If this combination would result in an `<empty string>` then the identifier becomes `total`.

Valid examples are:

- `total`
- `mainEngineStarboard`
- `portAft2`
- `starboard6`
- `5`

## 2. Paths for cargo

Current SignalK specs don't have all paths for information about cargo. GOSK uses the following paths to store information about cargo:

- `/vessels/<RegExp>/design/draft` _(extended)_
  - Description: The draft of the vessel
  - Units: m (Meter)
  - Object value with properties:
    - current (m), current average draft of the vessel
    - currentPort (m`[]`), list of drafts measured from forward to aft, the 0th element is the average of the list
    - currentStarboard (m`[]`), list of drafts measured from forward to aft, the 0th element is the average of the list
- `/vessels/<RegExp>/navigation/attitude` _(existing)_
  - Description: Vessel attitude: roll, pitch and yaw
  - Object value with properties
    - roll (rad)
    - pitch (rad)
    - yaw (rad)
- `/vessels/<RegExp>/cargo/total/capacity` _(new)_
  - Units: kg (Kilogram)
  - Description: Amount of cargo that can be loaded in the vessel
- `/vessels/<RegExp>/cargo/total/currentLevel` _(new)_
  - Units: ratio (Ratio)
  - Description: Amount of cargo loaded in the vessel in relation to the capacity
- `/vessels/<RegExp>/cargo/tank/<RegExp>/` _(new)_
  - Description: Tanks are used to store gas, liquid or powder cargo
- `/vessels/<RegExp>/cargo/tank/<RegExp>/capacity` _(new)_
  - Units: m3 (Cubic meter)
  - Description: Capacity of the tank
- `/vessels/<RegExp>/cargo/tank/<RegExp>/currentLevel` _(new)_
  - Units: ratio (Ratio)
  - Description: Amount of cargo loaded in the tank in relation to the capacity
- `/vessels/<RegExp>/cargo/tank/<RegExp>/density` _(new)_
  - Units: kg/m3 (undefined)
  - Description: Density of the cargo in the tank
- `/vessels/<RegExp>/cargo/tank/<RegExp>/temperature` _(new)_
  - Units: K (Kelvin)
  - Description: Temperature of the cargo in the tank
- `/vessels/<RegExp>/cargo/hold/<RegExp>/` _(new)_
  - Description: Holds are used to bulk, general cargo or containers
- `/vessels/<RegExp>/cargo/hold/<RegExp>/capacity` _(new)_
  - Units: kg (Kilogram)
  - Description: Capacity of the hold
- `/vessels/<RegExp>/cargo/hold/<RegExp>/currentLevel` _(new)_
  - Units: ratio (Ratio)
  - Description: Amount of cargo loaded in the hold in relation to the capacity

## 3. Events

Current SignalK specs don't have paths for events. GOSK uses the following paths to store information about events:

- `/vessels/<RegExp>/event/leg` _(new)_
  - Description: Information about a leg, a leg start when a vessels start/stops moving and ends when a vessel stops/start moving.
- `/vessels/<RegExp>/event/leg/type` _(new)_
  - Description: Type of leg
  - Enum values:
    - `Sailing` (the vessel is sailing with cargo, also used when cargo level is not known)
    - `SailingEmpty` (the vessel is sailing and doesn't have cargo)
    - `Moored` (the vessel is moored)
    - `Waiting` (the vessel is waiting for a lock or bridge)
- `/vessels/<RegExp>/event/leg/duration` _(new)_
  - Description: Duration of the leg
  - Units: s (Second)
- `/vessels/<RegExp>/event/leg/distance` _(new)_
  - Description: Distance traveled in the leg
  - Object value with properties:
    - overGround (m), integration of speedOverGround over time
    - throughWater (m), integration of speedThroughWater over time
    - greatCircle (m), <https://en.wikipedia.org/wiki/Great-circle_distance>
- `/vessels/<RegExp>/event/leg/position` _(new)_
  - Description: Position at the end of the leg
  - Object value with properties:
    - longitude (deg)
    - latitude (deg)
    - altitude (m)
- `/vessels/<RegExp>/event/leg/fuelConsumption` _(new)_
  - Description: Fuel used in the leg
  - Units: l (Liter)
- `/vessels/<RegExp>/event/leg/cargo` _(new)_
  - Description: Amount of cargo at the end of the leg
  - Units: kg (Kilogram)
- `/vessels/<RegExp>/event/loading` _(new)_
  - Description: Information about loading and unloading of cargo on a vessel, a loading event is recorded when the amount of cargo in a vessel has significantly changed.
- `/vessels/<RegExp>/event/loading/duration` _(new)_
  - Description: Duration of the loading/unloading event
  - Units: s (Second)
- `/vessels/<RegExp>/event/loading/position` _(new)_
  - Description: Position at the end of the loading/unloading event
  - Object value with properties:
    - longitude (deg)
    - latitude (deg)
    - altitude (m)
- `/vessels/<RegExp>/event/loading/fuelConsumption` _(new)_
  - Description: Fuel used during the loading/unloading event
  - Units: l (Liter)
- `/vessels/<RegExp>/event/loading/cargo` _(new)_
  - Description: Amount of cargo at the end of the loading/unloading event
  - Units: kg (Kilogram)
- `/vessels/<RegExp>/event/loading/delta` _(new)_
  - Description: Amount of cargo that has been loaded or unloaded, positive value for loaded cargo and negative value for unloaded cargo
  - Units: kg (Kilogram)
- `/vessels/<RegExp>/event/bunkering` _(new)_
  - Description: Information about bunkering of fuel on a vessel, a bunkering event is recorded when the amount of fuel in a vessel has significantly changed.
- `/vessels/<RegExp>/event/bunkering/duration` _(new)_
  - Description: Duration of the loading/unloading event
  - Units: s (Second)
- `/vessels/<RegExp>/event/bunkering/position` _(new)_
  - Description: Position at the end of the loading/unloading event
  - Object value with properties:
    - longitude (deg)
    - latitude (deg)
    - altitude (m)
- `/vessels/<RegExp>/event/loading/currentVolume` _(new)_
  - Description: Amount of fuel (total of all tanks) at the end of the bunkering event
  - Units: m3 (Cubic meter)
- `/vessels/<RegExp>/event/bunkering/delta` _(new)_
  - Description: Amount of fuel that has been bunkered, positive value for loaded fuel and negative value for unloaded fuel
  - Units: m3 (Cubic meter)
