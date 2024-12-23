import React, { useState, useEffect } from 'react';
import { useJaxClient } from '../client';
import { GetDexResponse } from '../generated/dex_pb';

interface DexViewerProps {
  host: string;
  underlyingAsset: string;
  startStrike?: number;
  endStrike?: number;
}

export const DexViewer: React.FC<DexViewerProps> = ({
  host,
  underlyingAsset,
  startStrike,
  endStrike,
}) => {
  const [data, setData] = useState<GetDexResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const jaxClient = useJaxClient({ host });

  useEffect(() => {
    const fetchData = async () => {
      setLoading(true);
      setError(null);

      try {
        const response = await jaxClient.getDex({
          underlyingAsset,
          startStrikePrice: startStrike,
          endStrikePrice: endStrike,
        });
        setData(response);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'An error occurred');
      } finally {
        setLoading(false);
      }
    };

    fetchData();
  }, [jaxClient, underlyingAsset, startStrike, endStrike]);

  if (loading) {
    return <div>Loading...</div>;
  }

  if (error) {
    return <div>Error: {error}</div>;
  }

  if (!data) {
    return null;
  }

  return (
    <div>
      <h2>DEX Data for {underlyingAsset}</h2>
      <p>Spot Price: {data.getSpotPrice()}</p>
      <div>
        {Object.entries(data.getStrikePricesMap().toObject()).map(([strike, expDates]) => (
          <div key={strike}>
            <h3>Strike: {strike}</h3>
            {Object.entries(expDates.getExpirationDatesMap().toObject()).map(([date, optTypes]) => (
              <div key={date}>
                <h4>Expiration: {date}</h4>
                {Object.entries(optTypes.getOptionTypesMap().toObject()).map(([type, dex]) => (
                  <div key={type}>
                    {type}: {dex.getValue()}
                  </div>
                ))}
              </div>
            ))}
          </div>
        ))}
      </div>
    </div>
  );
}; 