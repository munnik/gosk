package signalkws

type cacheType struct {
	content map[string]map[string]deltaMessage // map of context to a map of path to a delta
}

func (c *cacheType) injectOrUpdate(m deltaMessage) {
	if c.content == nil {
		c.content = make(map[string]map[string]deltaMessage)
	}
	if _, ok := c.content[m.Context]; !ok {
		c.content[m.Context] = make(map[string]deltaMessage)
	}
	for _, update := range m.Updates {
		for _, value := range update.Values {
			c.content[m.Context][value.Path] = deltaMessage{
				Context: m.Context,
				Updates: []updateSection{
					{
						Source:    update.Source,
						Timestamp: update.Timestamp,
						Values: []valueSection{
							{
								Path:  value.Path,
								Value: value.Value,
							},
						},
					},
				},
			}
		}
	}
}

func (c *cacheType) retrieveAll() []deltaMessage {
	if c.content == nil {
		return []deltaMessage{}
	}

	result := make([]deltaMessage, 0)
	for _, context := range c.content {
		for _, delta := range context {
			result = append(result, delta)
		}
	}

	return result
}
