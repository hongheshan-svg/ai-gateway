include $(TOPDIR)/rules.mk

LUCI_TITLE:=LuCI support for AI Gateway
LUCI_DESCRIPTION:=Web management interface for AI Gateway - configure AI API providers, identity normalization, and certificate management.
LUCI_DEPENDS:=+ai-gateway
LUCI_PKGARCH:=all

PKG_NAME:=luci-app-ai-gateway
PKG_VERSION:=0.1.0
PKG_RELEASE:=1
PKG_LICENSE:=MIT

include $(TOPDIR)/feeds/luci/luci.mk

define Package/luci-app-ai-gateway/conffiles
/etc/config/ai-gateway
endef

# call BuildPackage - OpenWrt buildroot signature
