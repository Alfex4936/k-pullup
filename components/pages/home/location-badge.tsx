"use client";

import Badge from "@common/badge";
import LocationIcon from "@icons/location-icon";
import useGeolocationStore from "@store/useGeolocationStore";

const LocationBadge = () => {
  const { region, geoLocationError } = useGeolocationStore();

  if (!region || geoLocationError) {
    return (
      <Badge
        text="위치 정보 없음"
        icon={<LocationIcon size={20} className="fill-primary-dark" />}
      />
    );
  }

  const { region_2depth_name, region_3depth_name, address_name } = region;
  const title =
    region_2depth_name !== "" || region_3depth_name !== ""
      ? `${region_2depth_name} ${region_3depth_name}`
      : address_name;

  return (
    <Badge
      text={title as string}
      icon={<LocationIcon size={20} className="fill-primary-dark" />}
    />
  );
};

export default LocationBadge;